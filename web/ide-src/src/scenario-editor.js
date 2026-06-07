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

// Extract scenario ID from path: /admin/scenarios/{id}/edit
const SCENARIO_ID = location.pathname.split('/')[3]
const API = `/api/admin/scenarios/${SCENARIO_ID}`

let editor = null
let currentPath = null
let dirty = false
let previewPane = null

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
  wordWrap: 'off',
  quickSuggestions: { other: true, comments: false, strings: true },
  suggestOnTriggerCharacters: true,
})
editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, saveFile)
editor.onDidChangeModelContent(() => {
  if (!dirty) { dirty = true; updateBar() }
  if (currentPath?.endsWith('.md')) schedulePreviewUpdate()
})

// ── File tree ────────────────────────────────────────────────────────────────
async function loadTree() {
  const res = await fetch(`${API}/files`)
  if (!res.ok) { document.getElementById('tree').innerHTML = '<div style="padding:8px;color:var(--danger);font-size:12px">Failed to load files</div>'; return }
  const paths = await res.json()
  renderTree(paths || [])
}

function renderTree(paths) {
  const container = document.getElementById('tree')
  container.innerHTML = ''

  // Group into dirs
  const dirs = {}
  const rootFiles = []
  for (const p of paths) {
    const parts = p.split('/')
    if (parts.length === 1) {
      rootFiles.push(p)
    } else {
      const dir = parts[0]
      dirs[dir] = dirs[dir] || []
      dirs[dir].push(p)
    }
  }

  // Root files first
  for (const p of rootFiles) appendFileItem(container, p, 0)

  // Dirs
  for (const [dir, files] of Object.entries(dirs).sort()) {
    const dirItem = document.createElement('div')
    dirItem.className = 'tree-item'
    dirItem.style.paddingLeft = '8px'
    const arrow = document.createElement('span')
    arrow.className = 'dir-arrow'
    arrow.textContent = '▸'
    const badge = document.createElement('span')
    badge.className = 'fbadge fb-dir'
    badge.textContent = '📁'
    badge.style.background = 'none'
    const name = document.createElement('span')
    name.className = 'name'
    name.textContent = dir
    dirItem.appendChild(arrow)
    dirItem.appendChild(badge)
    dirItem.appendChild(name)
    container.appendChild(dirItem)

    const children = document.createElement('div')
    children.className = 'tree-children open'
    for (const p of files.sort()) appendFileItem(children, p, 1)
    container.appendChild(children)

    dirItem.onclick = () => {
      const open = children.classList.toggle('open')
      arrow.textContent = open ? '▾' : '▸'
    }
  }
}

function appendFileItem(container, path, depth) {
  const item = document.createElement('div')
  item.className = 'tree-item'
  item.style.paddingLeft = `${8 + depth * 14}px`
  item.dataset.path = path

  const { text, cls } = fileIcon(path.split('/').pop())
  const badge = document.createElement('span')
  badge.className = `fbadge ${cls}`
  badge.textContent = text
  const name = document.createElement('span')
  name.className = 'name'
  name.textContent = path.split('/').pop()
  item.appendChild(badge)
  item.appendChild(name)
  container.appendChild(item)
  item.onclick = (ev) => { ev.stopPropagation(); openFile(path) }
}

function fileIcon(name) {
  const ext = name.includes('.') ? name.split('.').pop().toLowerCase() : name.toLowerCase()
  const map = {
    go: { text: 'GO', cls: 'fb-go' }, js: { text: 'JS', cls: 'fb-js' },
    ts: { text: 'TS', cls: 'fb-ts' }, py: { text: 'PY', cls: 'fb-py' },
    sh: { text: 'SH', cls: 'fb-sh' }, yaml: { text: 'YAML', cls: 'fb-yaml' },
    yml: { text: 'YAML', cls: 'fb-yaml' }, json: { text: 'JSON', cls: 'fb-json' },
    md: { text: 'MD', cls: 'fb-md' }, css: { text: 'CSS', cls: 'fb-css' },
    tf: { text: 'TF', cls: 'fb-tf' }, hcl: { text: 'HCL', cls: 'fb-tf' },
  }
  return map[ext] || { text: ext.slice(0, 4).toUpperCase() || '·', cls: 'fb-file' }
}

// ── Open / save ──────────────────────────────────────────────────────────────
async function openFile(path) {
  if (dirty && currentPath) {
    if (!confirm(`Discard unsaved changes to ${currentPath.split('/').pop()}?`)) return
  }
  const res = await fetch(`${API}/files/${path}`)
  if (!res.ok) { alert('Cannot open file'); return }
  const text = await res.text()

  currentPath = path
  dirty = false

  document.getElementById('no-file').style.display = 'none'
  document.getElementById('editor-container').style.display = 'block'
  document.getElementById('btn-save').disabled = false

  const lang = getLang(path)
  const model = monaco.editor.createModel(text, lang, monaco.Uri.file(`/${SCENARIO_ID}/${path}`))
  const old = editor.getModel()
  editor.setModel(model)
  if (old) old.dispose()

  // Show/hide preview pane for markdown
  const isMarkdown = path.endsWith('.md')
  document.getElementById('preview-pane').style.display = isMarkdown ? '' : 'none'
  document.getElementById('editor-container').style.flex = isMarkdown ? '1' : ''
  if (isMarkdown) updatePreview(text)

  updateBar()
  highlightActive(path)
}

async function saveFile() {
  if (!currentPath || !editor) return
  const content = editor.getValue()

  // For shell scripts, lint first
  if (currentPath.endsWith('.sh')) {
    const lintRes = await fetch('/api/admin/lint/shell', {
      method: 'POST', body: content,
      headers: { 'Content-Type': 'text/plain' },
    })
    const lintData = await lintRes.json()
    const diags = lintData.shellcheck || lintData
    if (Array.isArray(diags) && diags.length > 0) {
      setShellcheckMarkers(diags)
      setStatus(`shellcheck: ${diags.length} issue(s) — fix before saving`)
      return
    }
    monaco.editor.setModelMarkers(editor.getModel(), 'shellcheck', [])
  }

  const btn = document.getElementById('btn-save')
  btn.disabled = true
  setStatus('Saving…')
  try {
    const res = await fetch(`${API}/files/${currentPath}`, {
      method: 'PUT', body: content,
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
    })
    if (res.status === 422) {
      const data = await res.json()
      setShellcheckMarkers(data.shellcheck || [])
      setStatus(`shellcheck: save blocked`)
    } else if (res.ok) {
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

function setShellcheckMarkers(diags) {
  const model = editor.getModel()
  if (!model) return
  const markers = diags.map(d => ({
    severity: d.level === 'error' ? monaco.MarkerSeverity.Error : monaco.MarkerSeverity.Warning,
    startLineNumber: d.line, startColumn: d.column,
    endLineNumber: d.endLine || d.line, endColumn: d.endColumn || d.column + 1,
    message: `[SC${d.code}] ${d.message}`,
    source: 'shellcheck',
  }))
  monaco.editor.setModelMarkers(model, 'shellcheck', markers)
}

// ── Markdown preview ─────────────────────────────────────────────────────────
let previewTimer = null
function schedulePreviewUpdate() {
  clearTimeout(previewTimer)
  previewTimer = setTimeout(() => updatePreview(editor.getValue()), 300)
}

function updatePreview(md) {
  // Minimal markdown render — convert headings, bold, code, links
  const html = md
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/^### (.+)$/gm, '<h3>$1</h3>')
    .replace(/^## (.+)$/gm, '<h2>$1</h2>')
    .replace(/^# (.+)$/gm, '<h1>$1</h1>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/```(\w*)\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>')
    .replace(/^\- (.+)$/gm, '<li>$1</li>')
    .replace(/\n\n/g, '<br><br>')
  document.getElementById('preview-content').innerHTML = html
}

// ── Helpers ──────────────────────────────────────────────────────────────────
function updateBar() {
  const el = document.getElementById('filename')
  el.textContent = currentPath || 'No file open'
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
  const ext = path.split('.').pop().toLowerCase()
  return ({ go: 'go', js: 'javascript', ts: 'typescript', py: 'python',
    sh: 'shell', bash: 'shell', yaml: 'yaml', yml: 'yaml',
    json: 'json', md: 'markdown', css: 'css', tf: 'hcl', hcl: 'hcl',
  })[ext] || 'plaintext'
}

// expose for inline HTML handlers
window.saveFile = saveFile

// ── Boot ─────────────────────────────────────────────────────────────────────
loadTree()
