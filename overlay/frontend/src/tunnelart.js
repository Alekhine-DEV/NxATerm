// Ilustrações da janela de túneis (desenhos próprios, em SVG).
const svg = (vb, body) => `<svg viewBox="${vb}" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">${body}</svg>`

const SKIN = '#e9a23b'
const BODY = '#5d7ea8'
const BODY2 = '#47678f'

/** Pessoas (clientes). */
export const people = () => svg('0 0 150 100', `
  <g fill="${BODY2}"><path d="M30 60c-14 0-22 8-22 20v14h44V80c0-12-8-20-22-20z"/><path d="M120 60c-14 0-22 8-22 20v14h44V80c0-12-8-20-22-20z"/></g>
  <g fill="${SKIN}"><circle cx="30" cy="44" r="12"/><circle cx="120" cy="44" r="12"/></g>
  <g fill="${BODY}"><path d="M75 50c-20 0-31 11-31 27v17h62V77c0-16-11-27-31-27z"/></g>
  <circle cx="75" cy="30" r="16" fill="${SKIN}"/>`)

/** Servidor com o ícone de corrente (destino ou origem remota). */
export const server = (led = '#3fb97a') => svg('0 0 130 130', `
  <path d="M12 70l53-26 53 26v30c0 8-24 18-53 18S12 108 12 100z" fill="#4c5966"/>
  <path d="M12 70l53 26 53-26-53-26z" fill="#6d7d8c"/>
  <path d="M12 70v30c0 8 24 18 53 18V96z" fill="#3f4b57"/>
  <circle cx="100" cy="102" r="5" fill="${led}"/>
  <g fill="none" stroke="#5aa9e6" stroke-width="9" stroke-linecap="round">
    <rect x="30" y="8" width="38" height="22" rx="11" transform="rotate(-38 49 19)"/>
    <rect x="60" y="22" width="38" height="22" rx="11" transform="rotate(-38 79 33)"/>
  </g>`)

/** Computador do usuário (com um terminal na tela). */
export const computer = () => svg('0 0 150 120', `
  <rect x="10" y="6" width="130" height="86" rx="7" fill="#37424d"/>
  <rect x="17" y="13" width="116" height="72" rx="3" fill="#8fc4ef"/>
  <circle cx="52" cy="40" r="13" fill="#3a3f45"/>
  <path d="M28 85c0-17 11-26 24-26s24 9 24 26z" fill="#6aa04a"/>
  <rect x="82" y="26" width="46" height="34" rx="3" fill="#2b2f35"/>
  <path d="M90 36l8 6-8 6M102 50h12" fill="none" stroke="#e6e6e6" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M60 92h30l6 18H54z" fill="#4c5966"/><rect x="42" y="108" width="66" height="8" rx="4" fill="#6d7d8c"/>`)

/** Firewall: parede de tijolos e chama. */
export const firewall = () => svg('0 0 140 130', `
  <path d="M70 4c6 18 24 26 24 48 0 14-8 22-16 26 2-8-2-14-8-20-2 6-8 10-8 18-10-4-18-12-18-26 0-14 10-20 14-30 6 6 6 12 8 16 6-10 4-22 4-32z" fill="#f26b21"/>
  <path d="M70 34c3 10 14 16 14 30 0 9-6 15-14 16-8-1-14-7-14-16 0-10 8-14 14-30z" fill="#ffb02e"/>
  <rect x="6" y="78" width="128" height="46" rx="3" fill="#7a8591"/>
  <g stroke="#2f3943" stroke-width="3" fill="none">
    <path d="M6 93h128M6 108h128"/>
    <path d="M38 78v15M92 78v15M64 93v15M20 93v15M108 93v15M38 108v16M92 108v16"/>
  </g>`)

/** Servidor SSH: tela azul com setas nos dois sentidos. */
export const sshServer = () => svg('0 0 150 120', `
  <rect x="10" y="6" width="130" height="82" rx="6" fill="#37424d"/>
  <rect x="17" y="13" width="116" height="68" rx="3" fill="#a9d3f5"/>
  <g fill="#2f62b5"><path d="M30 38h40v-10l22 16-22 16V50H30z"/><path d="M120 66H80v10L58 60l22-16v10h40z" opacity=".92"/></g>
  <path d="M60 88h30l4 14H56z" fill="#4c5966"/>
  <rect x="38" y="100" width="74" height="12" rx="6" fill="#1e9aa8"/>`)
