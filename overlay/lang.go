package main

import "sync/atomic"

// Idioma da interface, informado pelo frontend. O backend o usa só nos textos que ele mesmo mostra:
// títulos dos diálogos de arquivo e o menu da bandeja. As mensagens de erro são traduzidas na interface.
var uiLang atomic.Value // "pt", "en" ou "es"

// goText: português -> {inglês, espanhol}.
var goText = map[string][2]string{
	"Escolha a chave privada":                  {"Choose the private key", "Elija la clave privada"},
	"Escolha a imagem de fundo":                {"Choose the background image", "Elija la imagen de fondo"},
	"Escolha o arquivo de sessões":             {"Choose the sessions file", "Elija el archivo de sesiones"},
	"Escolha a pasta para enviar":              {"Choose the folder to upload", "Elija la carpeta para enviar"},
	"Escolha os arquivos para enviar":          {"Choose the files to upload", "Elija los archivos para enviar"},
	"Escolha a pasta onde salvar":              {"Choose the folder to save to", "Elija la carpeta donde guardar"},
	"Escolha os arquivos para enviar (Zmodem)": {"Choose the files to send (Zmodem)", "Elija los archivos para enviar (Zmodem)"},
	"Salvar arquivos recebidos em (Zmodem)":    {"Save received files to (Zmodem)", "Guardar los archivos recibidos en (Zmodem)"},
	"Escolha os arquivos para enviar (Trzsz)":  {"Choose the files to send (Trzsz)", "Elija los archivos para enviar (Trzsz)"},
	"Escolha a pasta para enviar (Trzsz)":      {"Choose the folder to send (Trzsz)", "Elija la carpeta para enviar (Trzsz)"},
	"Salvar arquivos recebidos em (Trzsz)":     {"Save received files to (Trzsz)", "Guardar los archivos recibidos en (Trzsz)"},
	"Mostrar o NxATerm":                        {"Show NxATerm", "Mostrar NxATerm"},
	"Sair":                                     {"Quit", "Salir"},
}

// tr devolve o texto no idioma atual (o próprio texto em português, se não houver tradução).
func tr(pt string) string {
	l, _ := uiLang.Load().(string)
	t, ok := goText[pt]
	if !ok {
		return pt
	}
	switch l {
	case "en":
		return t[0]
	case "es":
		return t[1]
	}
	return pt
}

// SetLanguage é chamado pela interface quando o idioma muda ("pt", "en" ou "es").
func (a *TermService) SetLanguage(lang string) {
	switch lang {
	case "pt", "en", "es":
	default:
		return
	}
	uiLang.Store(lang)
	a.retranslateTray()
}
