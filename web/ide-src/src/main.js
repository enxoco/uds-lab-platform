import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import YamlWorker from 'monaco-yaml/yaml.worker?worker'
import * as monaco from 'monaco-editor'
import { configureMonacoYaml } from 'monaco-yaml'

self.MonacoEnvironment = {
  getWorker(_, label) {
    if (label === 'yaml') return new YamlWorker()
    return new EditorWorker()
  },
}

configureMonacoYaml(monaco, {
  enableSchemaRequest: true,
  hover: true,
  completion: true,
  validate: true,
  format: true,
})

const SESSION = location.pathname.split('/')[2]
const API = `/ide-api/${SESSION}`

let editor = null
let currentPath = null
let dirty = false

// ── Monaco init ──────────────────────────────────────────────────────────────
document.getElementById('editor-container').style.display = 'none'
editor = monaco.editor.create(document.getElementById('editor-container'), {
  theme: 'vs-dark',
  minimap: { enabled: false },
  fontSize: 13,
  fontFamily: '"SFMono-Regular", Consolas, "Liberation Mono", monospace',
  lineNumbers: 'on',
  scrollBeyondLastLine: false,
  automaticLayout: true,
  renderWhitespace: 'selection',
  tabSize: 2,
  quickSuggestions: { other: true, comments: false, strings: true },
  suggestOnTriggerCharacters: true,
  acceptSuggestionOnEnter: 'on',
})
editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, saveFile)
editor.onDidChangeModelContent(() => {
  if (!dirty) { dirty = true; updateBar() }
})

// ── File tree ────────────────────────────────────────────────────────────────
const ROOT_NOISE = new Set(['snap', 'proc', 'sys', 'dev', 'run'])

function navigateTo(path) {
  path = path.trim() || '/root'
  document.getElementById('tree-path-input').value = path
  const container = document.getElementById('tree')
  container.innerHTML = ''
  loadTree(path, container, 0)
}

async function loadTree(path, container, depth = 0) {
  const res = await fetch(`${API}/files?path=${encodeURIComponent(path)}`)
  if (!res.ok) return
  const entries = await res.json()
  for (const e of entries) {
    if (depth === 0 && ROOT_NOISE.has(e.name)) continue
    const item = document.createElement('div')
    item.className = 'tree-item'
    item.style.paddingLeft = `${8 + depth * 14}px`
    item.dataset.path = e.path
    item.dataset.type = e.type

    const badge = document.createElement('span')
    const name = document.createElement('span')
    name.className = 'name'
    name.textContent = e.name

    if (e.type === 'dir') {
      const arrow = document.createElement('span')
      arrow.className = 'dir-arrow'
      arrow.textContent = '▸'
      badge.className = 'fbadge fb-dir'
      badge.textContent = '📁'
      badge.style.background = 'none'
      item.appendChild(arrow)
      item.appendChild(badge)
      item.appendChild(name)
      container.appendChild(item)

      const children = document.createElement('div')
      children.className = 'tree-children'
      container.appendChild(children)
      item.onclick = (ev) => {
        ev.stopPropagation()
        const open = children.classList.toggle('open')
        arrow.textContent = open ? '▾' : '▸'
        if (open && children.children.length === 0) {
          loadTree(e.path, children, depth + 1)
        }
      }
    } else {
      const { text, cls } = fileIcon(e.name)
      badge.className = `fbadge ${cls}`
      badge.textContent = text
      item.appendChild(badge)
      item.appendChild(name)
      container.appendChild(item)
      item.onclick = (ev) => { ev.stopPropagation(); openFile(e.path) }
    }
  }
}

function fileIcon(name) {
  const ext = name.includes('.') ? name.split('.').pop().toLowerCase() : name.toLowerCase()
  const map = {
    go:         { text: 'GO',   cls: 'fb-go'   },
    js:         { text: 'JS',   cls: 'fb-js'   },
    jsx:        { text: 'JSX',  cls: 'fb-js'   },
    ts:         { text: 'TS',   cls: 'fb-ts'   },
    tsx:        { text: 'TSX',  cls: 'fb-ts'   },
    py:         { text: 'PY',   cls: 'fb-py'   },
    sh:         { text: 'SH',   cls: 'fb-sh'   },
    bash:       { text: 'SH',   cls: 'fb-sh'   },
    yaml:       { text: 'YAML', cls: 'fb-yaml' },
    yml:        { text: 'YAML', cls: 'fb-yaml' },
    json:       { text: 'JSON', cls: 'fb-json' },
    md:         { text: 'MD',   cls: 'fb-md'   },
    html:       { text: 'HTML', cls: 'fb-html' },
    css:        { text: 'CSS',  cls: 'fb-css'  },
    tf:         { text: 'TF',   cls: 'fb-tf'   },
    hcl:        { text: 'HCL',  cls: 'fb-tf'   },
    rs:         { text: 'RS',   cls: 'fb-rs'   },
    toml:       { text: 'TOML', cls: 'fb-json' },
    dockerfile: { text: 'DF',   cls: 'fb-sh'   },
    c:          { text: 'C',    cls: 'fb-rs'   },
    cpp:        { text: 'C++',  cls: 'fb-rs'   },
  }
  const byName = {
    dockerfile:   { text: 'DF',  cls: 'fb-sh'   },
    makefile:     { text: 'MK',  cls: 'fb-sh'   },
    '.gitignore': { text: 'GIT', cls: 'fb-md'   },
    '.env':       { text: 'ENV', cls: 'fb-yaml' },
  }
  return map[ext] || byName[name.toLowerCase()] || { text: ext.slice(0, 4).toUpperCase() || '·', cls: 'fb-file' }
}

// ── Open / save ──────────────────────────────────────────────────────────────
async function openFile(path) {
  if (dirty && currentPath) {
    if (!confirm(`Discard unsaved changes to ${currentPath.split('/').pop()}?`)) return
  }
  const res = await fetch(`${API}/files?path=${encodeURIComponent(path)}`)
  if (!res.ok) { alert('Cannot open file'); return }
  const text = await res.text()

  currentPath = path
  dirty = false

  document.getElementById('no-file').style.display = 'none'
  document.getElementById('editor-container').style.display = 'block'
  document.getElementById('btn-save').disabled = false

  const lang = getLang(path)
  const model = monaco.editor.createModel(text, lang, monaco.Uri.file(path))
  const old = editor.getModel()
  editor.setModel(model)
  if (old) old.dispose()

  updateBar()
  highlightActive(path)
}

async function saveFile() {
  if (!currentPath || !editor) return
  const content = editor.getValue()
  const btn = document.getElementById('btn-save')
  btn.disabled = true
  setStatus('Saving…')
  try {
    const res = await fetch(`${API}/files?path=${encodeURIComponent(currentPath)}`, {
      method: 'PUT',
      body: content,
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
    })
    if (res.ok) {
      dirty = false
      updateBar()
      setStatus('Saved')
      setTimeout(() => setStatus(''), 1500)
    } else {
      setStatus('Save failed')
    }
  } catch { setStatus('Save failed') }
  btn.disabled = false
}

// ── Helpers ──────────────────────────────────────────────────────────────────
function updateBar() {
  const el = document.getElementById('filename')
  el.textContent = currentPath ? currentPath.replace('/root/', '~/') : 'No file open'
  el.className = 'filename' + (dirty ? ' dirty' : '')
}

function setStatus(msg) {
  document.getElementById('save-status').textContent = msg
}

function highlightActive(path) {
  document.querySelectorAll('.tree-item').forEach(el => {
    el.classList.toggle('active', el.dataset.path === path)
  })
}

function getLang(path) {
  const name = path.split('/').pop()
  const ext = name.includes('.') ? name.split('.').pop().toLowerCase() : name.toLowerCase()
  return ({
    go: 'go', js: 'javascript', ts: 'typescript', jsx: 'javascript', tsx: 'typescript',
    py: 'python', sh: 'shell', bash: 'shell', yaml: 'yaml', yml: 'yaml',
    json: 'json', md: 'markdown', html: 'html', css: 'css', toml: 'ini',
    tf: 'hcl', hcl: 'hcl', rs: 'rust', c: 'c', cpp: 'cpp', java: 'java',
    rb: 'ruby', sql: 'sql', dockerfile: 'dockerfile',
  })[ext] || 'plaintext'
}

// expose for ide.html inline handlers
window.navigateTo = navigateTo
window.saveFile = saveFile

// ── Boot ─────────────────────────────────────────────────────────────────────
navigateTo('/root')
