package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"sync"
	"time"

	"nxaterm/internal/xfer"
)

// TransferInfo é o estado de uma transferência, enviado ao frontend.
type TransferInfo struct {
	ID      string `json:"id"`
	TabID   string `json:"tabId"`
	Name    string `json:"name"`
	Dir     string `json:"dir"` // up | down
	Src     string `json:"src"`
	Dst     string `json:"dst"`
	Done    int64  `json:"done"`
	Total   int64  `json:"total"`
	Files   int    `json:"files"`
	Skipped int    `json:"skipped"`
	State   string `json:"state"` // queued running conflict done error canceled paused
	Err     string `json:"err"`
	Speed   int64  `json:"speed"` // bytes por segundo
	Current string `json:"current"`
	// Scanning: a pasta ainda está sendo medida (o total é desconhecido).
	Scanning  bool `json:"scanning"`
	ScanFiles int  `json:"scanFiles"`
}

type conflictInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	SrcSize int64  `json:"srcSize"`
	DstSize int64  `json:"dstSize"`
	SrcTime int64  `json:"srcTime"`
	DstTime int64  `json:"dstTime"`
}

type conflictAnswer struct {
	action string
	all    bool
}

type xferJob struct {
	mu      sync.Mutex
	info    TransferInfo
	cancel  context.CancelFunc
	pausing bool
	policy  string

	src, dst         xfer.FS
	srcPath, dstPath string
	ex               xfer.Exec // comando no servidor (SSH); habilita o modo tar para pastas
	answer           chan conflictAnswer
}

func (j *xferJob) snapshot() TransferInfo {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.info
}

type xferState struct {
	mu    sync.Mutex
	seq   int
	jobs  map[string]*xferJob
	order []string
	sem   chan struct{}
}

func newXferState() *xferState {
	return &xferState{jobs: map[string]*xferJob{}, sem: make(chan struct{}, 3)}
}

func (a *TermService) emitJSON(name string, v any) {
	if js, err := json.Marshal(v); err == nil {
		a.app.Event.Emit(name, string(js))
	}
}

func (a *TermService) emitXfer(j *xferJob) { a.emitJSON("xfer:update", j.snapshot()) }

// enqueue cria a transferência e a coloca na fila (até 3 rodam ao mesmo tempo).
func (a *TermService) enqueue(tabID, dir string, src, dst xfer.FS, srcPath, dstPath, name, policy string, ex xfer.Exec) string {
	x := a.xf
	x.mu.Lock()
	x.seq++
	id := fmt.Sprintf("x%d", x.seq)
	j := &xferJob{
		policy: policy, src: src, dst: dst, srcPath: srcPath, dstPath: dstPath, ex: ex,
		answer: make(chan conflictAnswer, 1),
		info:   TransferInfo{ID: id, TabID: tabID, Name: name, Dir: dir, Src: srcPath, Dst: dstPath, State: "queued"},
	}
	x.jobs[id] = j
	x.order = append(x.order, id)
	x.mu.Unlock()
	a.emitXfer(j)
	go a.runJob(j)
	return id
}

func (a *TermService) runJob(j *xferJob) {
	ctx, cancel := context.WithCancel(context.Background())
	j.mu.Lock()
	j.cancel = cancel
	pol := j.policy
	j.mu.Unlock()
	defer cancel()

	select {
	case a.xf.sem <- struct{}{}:
		defer func() { <-a.xf.sem }()
	case <-ctx.Done():
		a.finishJob(j, ctx.Err(), xfer.Stats{})
		return
	}

	j.mu.Lock()
	j.info.State = "running"
	j.info.Err = ""
	j.mu.Unlock()
	a.emitXfer(j)

	stop := make(chan struct{})
	go func() {
		tk := time.NewTicker(250 * time.Millisecond)
		defer tk.Stop()
		var last int64
		lastT := time.Now()
		for {
			select {
			case <-stop:
				return
			case now := <-tk.C:
				j.mu.Lock()
				d := j.info.Done
				if dt := now.Sub(lastT).Seconds(); dt > 0 {
					j.info.Speed = int64(float64(d-last) / dt)
				}
				last, lastT = d, now
				j.mu.Unlock()
				a.emitXfer(j)
			}
		}
	}()

	progress := func(done, total int64, cur string) {
		j.mu.Lock()
		j.info.Done, j.info.Current = done, cur
		if total > 0 {
			j.info.Total = total
		}
		j.mu.Unlock()
	}
	if st, ok, err := a.tryTar(ctx, j, progress); ok {
		close(stop)
		a.finishJob(j, err, st)
		return
	}

	st, err := xfer.Copy(ctx, j.src, j.dst, j.srcPath, j.dstPath, xfer.Options{
		Policy: pol,
		Ask: func(c xfer.Conflict) (string, bool, error) {
			j.mu.Lock()
			j.info.State = "conflict"
			j.mu.Unlock()
			a.emitXfer(j)
			a.emitJSON("xfer:conflict", conflictInfo{
				ID: j.info.ID, Name: c.Name, Path: c.Path, SrcSize: c.SrcSize, DstSize: c.DstSize,
				SrcTime: c.SrcTime.Unix(), DstTime: c.DstTime.Unix(),
			})
			select {
			case ans := <-j.answer:
				j.mu.Lock()
				j.info.State = "running"
				j.mu.Unlock()
				return ans.action, ans.all, nil
			case <-ctx.Done():
				return "", false, ctx.Err()
			}
		},
		Scan: func(bytes int64, files int, final, ok bool) {
			j.mu.Lock()
			j.info.Scanning = !final
			j.info.ScanFiles = files
			if final && ok {
				j.info.Total = bytes
			}
			j.mu.Unlock()
		},
		Progress: progress,
	})
	close(stop)
	a.finishJob(j, err, st)
}

func (a *TermService) finishJob(j *xferJob, err error, st xfer.Stats) {
	j.mu.Lock()
	j.info.Files, j.info.Skipped = st.Files, st.Skipped
	j.info.Speed = 0
	j.info.Current = ""
	j.info.Scanning = false
	switch {
	case err == nil:
		j.info.State = "done"
		if j.info.Total > j.info.Done {
			j.info.Done = j.info.Total
		}
	case errors.Is(err, context.Canceled):
		if j.pausing {
			j.info.State = "paused"
		} else {
			j.info.State = "canceled"
		}
	case errors.Is(err, xfer.ErrCanceled):
		j.info.State = "canceled"
	default:
		j.info.State = "error"
		j.info.Err = err.Error()
	}
	j.pausing = false
	j.cancel = nil
	j.mu.Unlock()
	a.emitXfer(j)
}

func (a *TermService) job(id string) (*xferJob, error) {
	a.xf.mu.Lock()
	defer a.xf.mu.Unlock()
	j, ok := a.xf.jobs[id]
	if !ok {
		return nil, errors.New("transferência não encontrada")
	}
	return j, nil
}

// Transfers devolve todas as transferências (a mais antiga primeiro).
func (a *TermService) Transfers() []TransferInfo {
	a.xf.mu.Lock()
	list := make([]*xferJob, 0, len(a.xf.order))
	for _, id := range a.xf.order {
		list = append(list, a.xf.jobs[id])
	}
	a.xf.mu.Unlock()
	out := make([]TransferInfo, 0, len(list))
	for _, j := range list {
		out = append(out, j.snapshot())
	}
	return out
}

// AnswerConflict responde ao aviso "o arquivo já existe". Ações: overwrite,
// skip, rename, resume ou cancel; all aplica a escolha ao resto da transferência.
func (a *TermService) AnswerConflict(id, action string, all bool) error {
	j, err := a.job(id)
	if err != nil {
		return err
	}
	switch action {
	case xfer.Overwrite, xfer.Skip, xfer.Rename, xfer.Resume, xfer.Cancel:
	default:
		return fmt.Errorf("ação inválida: %q", action)
	}
	select {
	case j.answer <- conflictAnswer{action, all}:
	default:
	}
	return nil
}

// CancelTransfer cancela uma transferência em andamento ou na fila.
func (a *TermService) CancelTransfer(id string) error {
	j, err := a.job(id)
	if err != nil {
		return err
	}
	j.mu.Lock()
	c := j.cancel
	j.mu.Unlock()
	if c != nil {
		c()
	}
	return nil
}

// PauseTransfer interrompe mantendo o que já foi copiado; ResumeTransfer continua de onde parou.
func (a *TermService) PauseTransfer(id string) error {
	j, err := a.job(id)
	if err != nil {
		return err
	}
	j.mu.Lock()
	c := j.cancel
	j.pausing = true
	j.mu.Unlock()
	if c != nil {
		c()
	}
	return nil
}

// ResumeTransfer retoma uma transferência pausada, cancelada ou com erro,
// continuando os arquivos parciais (os completos são pulados).
func (a *TermService) ResumeTransfer(id string) error {
	j, err := a.job(id)
	if err != nil {
		return err
	}
	j.mu.Lock()
	switch j.info.State {
	case "paused", "canceled", "error":
	default:
		j.mu.Unlock()
		return nil
	}
	j.policy = xfer.Resume
	j.info.State = "queued"
	j.info.Err = ""
	j.mu.Unlock()
	a.emitXfer(j)
	go a.runJob(j)
	return nil
}

// RemoveTransfer tira uma transferência parada (concluída, com erro, cancelada ou pausada) da lista.
func (a *TermService) RemoveTransfer(id string) error {
	j, err := a.job(id)
	if err != nil {
		return err
	}
	switch j.snapshot().State {
	case "queued", "running", "conflict":
		return errors.New("cancele a transferência antes de remover")
	}
	x := a.xf
	x.mu.Lock()
	delete(x.jobs, id)
	for i, o := range x.order {
		if o == id {
			x.order = append(x.order[:i], x.order[i+1:]...)
			break
		}
	}
	x.mu.Unlock()
	a.app.Event.Emit("xfer:cleared", "")
	return nil
}

// ClearTransfers tira da lista as transferências que já terminaram.
func (a *TermService) ClearTransfers() {
	x := a.xf
	x.mu.Lock()
	keep := x.order[:0]
	for _, id := range x.order {
		switch x.jobs[id].snapshot().State {
		case "done", "canceled", "error":
			delete(x.jobs, id)
		default:
			keep = append(keep, id)
		}
	}
	x.order = keep
	x.mu.Unlock()
	a.app.Event.Emit("xfer:cleared", "")
}

func (a *TermService) stopTransfersOf(tabID string) {
	a.xf.mu.Lock()
	var cancels []context.CancelFunc
	for _, j := range a.xf.jobs {
		j.mu.Lock()
		if j.info.TabID == tabID && j.cancel != nil {
			cancels = append(cancels, j.cancel)
		}
		j.mu.Unlock()
	}
	a.xf.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func (a *TermService) stopAllTransfers() {
	a.xf.mu.Lock()
	var cancels []context.CancelFunc
	for _, j := range a.xf.jobs {
		j.mu.Lock()
		if j.cancel != nil {
			cancels = append(cancels, j.cancel)
		}
		j.mu.Unlock()
	}
	a.xf.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

// ------------------------------------------------------------------ pontos de entrada

// Upload envia arquivos/pastas locais para remoteDir. Devolve os IDs das transferências.
func (a *TermService) Upload(tabID string, localPaths []string, remoteDir string) ([]string, error) {
	t, err := a.term(tabID)
	if err != nil {
		return nil, err
	}
	rfs, err := t.files()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, lp := range localPaths {
		name := baseName(lp)
		ids = append(ids, a.enqueue(tabID, "up", xfer.Local{}, rfs, lp, rfs.Join(remoteDir, name), name, xfer.Ask, t.execFn()))
	}
	return ids, nil
}

// Download baixa itens remotos para a pasta local localDir.
func (a *TermService) Download(tabID string, remotePaths []string, localDir string) ([]string, error) {
	t, err := a.term(tabID)
	if err != nil {
		return nil, err
	}
	rfs, err := t.files()
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(localDir); err != nil || !fi.IsDir() {
		return nil, errors.New("a pasta de destino não existe")
	}
	var ids []string
	for _, rp := range remotePaths {
		name := pathpkg.Base(rp)
		ids = append(ids, a.enqueue(tabID, "down", rfs, xfer.Local{}, rp, filepath.Join(localDir, name), name, xfer.Ask, t.execFn()))
	}
	return ids, nil
}

// PickUpload abre o seletor de arquivos (ou de pasta, com dirs=true) e envia o que for escolhido.
func (a *TermService) PickUpload(tabID, remoteDir string, dirs bool) (int, error) {
	var paths []string
	var err error
	if dirs {
		var p string
		p, err = a.app.Dialog.OpenFile().SetTitle(tr("Escolha a pasta para enviar")).
			CanChooseDirectories(true).CanChooseFiles(false).PromptForSingleSelection()
		if p != "" {
			paths = []string{p}
		}
	} else {
		paths, err = a.app.Dialog.OpenFile().SetTitle(tr("Escolha os arquivos para enviar")).PromptForMultipleSelection()
	}
	if err != nil && !isCancel(err) {
		return 0, err
	}
	if err != nil || len(paths) == 0 {
		return 0, nil
	}
	ids, err := a.Upload(tabID, paths, remoteDir)
	return len(ids), err
}

// PickDownload pergunta a pasta de destino e baixa os itens remotos para ela.
func (a *TermService) PickDownload(tabID string, remotePaths []string) (int, error) {
	dir, err := a.app.Dialog.OpenFile().SetTitle(tr("Escolha a pasta onde salvar")).
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).PromptForSingleSelection()
	if err != nil && !isCancel(err) {
		return 0, err
	}
	if err != nil || dir == "" {
		return 0, nil
	}
	ids, err := a.Download(tabID, remotePaths, dir)
	return len(ids), err
}

// LocalCopy copia itens locais para outra pasta local, pela fila de transferências.
func (a *TermService) LocalCopy(paths []string, dstDir string) ([]string, error) {
	if fi, err := os.Stat(dstDir); err != nil || !fi.IsDir() {
		return nil, errors.New("a pasta de destino não existe")
	}
	var ids []string
	for _, p := range paths {
		name := filepath.Base(p)
		ids = append(ids, a.enqueue("", "up", xfer.Local{}, xfer.Local{}, p, filepath.Join(dstDir, name), name, xfer.Ask, nil))
	}
	return ids, nil
}

// LocalMove move itens locais para outra pasta (no mesmo disco; entre discos use copiar e apagar).
func (a *TermService) LocalMove(paths []string, dstDir string) error {
	for _, p := range paths {
		dst := filepath.Join(dstDir, filepath.Base(p))
		if dst == p {
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("já existe %q na pasta de destino", filepath.Base(p))
		}
		if err := os.Rename(p, dst); err != nil {
			return err
		}
	}
	return nil
}

// tryTar transfere uma pasta com muitos arquivos por um único fluxo tar (muito
// mais rápido que arquivo por arquivo pelo SFTP e bem mais leve para o disco do
// servidor). ok=false quando não se aplica (não é pasta, o destino já existe e
// precisa de tratamento de conflito, poucos arquivos, servidor sem tar…): aí o
// motor normal cuida de tudo.
func (a *TermService) tryTar(ctx context.Context, j *xferJob, progress func(done, total int64, cur string)) (st xfer.Stats, ok bool, err error) {
	if j.ex == nil {
		return
	}
	setScan := func(v bool) {
		j.mu.Lock()
		j.info.Scanning = v
		j.mu.Unlock()
		a.emitXfer(j)
	}
	var run func(total int64) (xfer.Stats, error)
	var total int64
	switch j.info.Dir {
	case "down":
		se, serr := j.src.Stat(j.srcPath)
		if serr != nil || !se.IsDir {
			return
		}
		if _, lerr := os.Lstat(j.dstPath); lerr == nil {
			return // já existe: o motor normal pergunta/retoma arquivo por arquivo
		}
		setScan(true)
		files, bytes, good, perr := xfer.TarProbe(ctx, j.ex, j.srcPath)
		setScan(false)
		if ctx.Err() != nil {
			return xfer.Stats{}, true, ctx.Err()
		}
		if perr != nil || !good || files < xfer.MinTarFiles {
			return
		}
		total = bytes
		run = func(total int64) (xfer.Stats, error) {
			return xfer.TarDownload(ctx, j.ex, j.srcPath, j.dstPath, total, progress)
		}
	case "up":
		fi, lerr := os.Stat(j.srcPath)
		if lerr != nil || !fi.IsDir() || pathpkg.Base(j.dstPath) != filepath.Base(j.srcPath) {
			return
		}
		if _, derr := j.dst.Stat(j.dstPath); !errors.Is(derr, fs.ErrNotExist) {
			return
		}
		files, bytes, serr := xfer.LocalScan(j.srcPath)
		if serr != nil || files < xfer.MinTarFiles {
			return
		}
		setScan(true)
		good, perr := xfer.HasTar(ctx, j.ex)
		setScan(false)
		if ctx.Err() != nil {
			return xfer.Stats{}, true, ctx.Err()
		}
		if perr != nil || !good {
			return
		}
		total = bytes
		run = func(total int64) (xfer.Stats, error) {
			return xfer.TarUpload(ctx, j.ex, j.srcPath, pathpkg.Dir(j.dstPath), pathpkg.Base(j.dstPath), total, progress)
		}
	default:
		return
	}
	st, err = run(total)
	return st, true, err
}
