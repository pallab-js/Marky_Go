import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { EventsOn } from '../wailsjs/runtime/runtime';

import './App.css';
import { MarkdownEditor } from './components/MarkdownEditor';
import { MarkdownPreview } from './components/MarkdownPreview';
import { api, type NoteDocument, type SearchResult, type Settings, type TreeNode, type VaultInfo } from './lib/api';


type SaveState = 'idle' | 'dirty' | 'saving' | 'saved' | 'conflict' | 'error';
type SaveOutcome = 'clean' | 'saved' | 'conflict' | 'error';

function withMarkdownExt(path: string): string {
  const trimmed = path.trim();
  return /\.md$/i.test(trimmed) ? trimmed : `${trimmed}.md`;
}

function flatten(nodes: TreeNode[], out: TreeNode[] = []): TreeNode[] {
  for (const n of nodes) {
    out.push(n);
    if (n.children?.length) flatten(n.children, out);
  }
  return out;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function renderSnippet(snippet: string): string {
  // SQLite snippet() uses private-use markers so literal [brackets] in note
  // text are not mistaken for search highlights.
  const safe = escapeHtml(snippet || '');
  return safe.replace(/\u0001([\s\S]*?)\u0002/g, '<mark>$1</mark>');
}

function TreeView({ nodes, selected, onSelect }: { nodes: TreeNode[]; selected?: string; onSelect: (n: TreeNode) => void }) {

  return (
    <ul className="tree">
      {nodes.map((n) => (
        <li key={n.path || n.absPath}>
          <button
            className={`tree-item ${n.isDir ? 'dir' : 'file'} ${selected === n.path ? 'selected' : ''}`}
            onClick={() => onSelect(n)}
            title={n.path || n.name}
          >
            <span className="tree-icon">{n.isDir ? '▸' : '•'}</span>
            <span className="tree-name">{n.name}</span>
          </button>
          {n.isDir && n.children?.length ? <TreeView nodes={n.children} selected={selected} onSelect={onSelect} /> : null}
        </li>
      ))}
    </ul>
  );
}

function App() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [vault, setVault] = useState<VaultInfo | null>(null);
  const [tree, setTree] = useState<TreeNode[]>([]);
  const [baseDoc, setBaseDoc] = useState<NoteDocument | null>(null);
  const [buffer, setBuffer] = useState('');
  const [status, setStatus] = useState<string>('No vault open');
  const [error, setError] = useState<string>('');
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [indexBusy, setIndexBusy] = useState(false);
  const [preview, setPreview] = useState(false);
  const [saveState, setSaveState] = useState<SaveState>('idle');
  const [conflictMsg, setConflictMsg] = useState('');

  const saveTimer = useRef<number | null>(null);
  const savePromise = useRef<Promise<SaveOutcome> | null>(null);
  const searchSeq = useRef(0);
  const baseRef = useRef<NoteDocument | null>(null);
  const bufferRef = useRef('');
  const queryRef = useRef('');

  baseRef.current = baseDoc;
  bufferRef.current = buffer;
  queryRef.current = query;

  const fileCount = useMemo(() => flatten(tree).filter((n) => !n.isDir).length, [tree]);
  const dirty = baseDoc ? buffer !== baseDoc.content : false;

  useEffect(() => {
    api.getSettings().then((s) => {
      setSettings(s);
      setPreview(!!s.previewDefault);
    }).catch(() => {});
  }, []);

  useEffect(() => {
    const q = query.trim();
    const seq = ++searchSeq.current;
    if (!vault || !q) {
      setResults([]);
      setSearching(false);
      return;
    }
    const t = setTimeout(async () => {
      if (seq !== searchSeq.current) return;
      setSearching(true);
      try {
        const found = await api.search(q, { limit: 50 });
        if (seq !== searchSeq.current) return;
        setResults(found);
        setError('');
      } catch (e) {
        if (seq !== searchSeq.current) return;
        setResults([]);
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        if (seq === searchSeq.current) setSearching(false);
      }
    }, 220);
    return () => clearTimeout(t);
  }, [query, vault]);

  useEffect(() => {
    if (!vault) return;
    const off = EventsOn('vault:changed', async () => {
      try {
        setTree(await api.getTree());
        const q = queryRef.current.trim();
        if (q) setResults(await api.search(q, { limit: 50 }));
        const doc = baseRef.current;
        if (doc) {
          const latest = await api.readNote(doc.path).catch(() => null);
          if (latest) {
            const isDirty = bufferRef.current !== doc.content;
            if (!isDirty && latest.contentHash !== doc.contentHash) {
              setBaseDoc(latest);
              setBuffer(latest.content);
              setStatus(`Reloaded external change: ${latest.path}`);
            } else if (isDirty && latest.contentHash !== doc.contentHash) {
              setSaveState('conflict');
              setConflictMsg('File changed on disk while you have unsaved edits. Reload, keep your version, or save a copy.');
              setStatus(`External change detected: ${doc.path}`);
            }
          } else {
            // File may have been deleted/moved externally.
            if (bufferRef.current !== doc.content) {
              setSaveState('conflict');
              setConflictMsg('The open file was removed or moved on disk. Save as copy to keep your edits, or close it.');
              setStatus(`File missing on disk: ${doc.path}`);
            } else {
              setBaseDoc(null);
              setBuffer('');
              setStatus(`Closed missing file: ${doc.path}`);
            }
          }
        }
        setStatus((s) => s.startsWith('Reloaded external') || s.startsWith('External change') || s.startsWith('File missing') || s.startsWith('Closed missing') ? s : 'Vault updated from filesystem');
      } catch {
        // ignore refresh errors
      }

    });
    return () => { if (typeof off === 'function') off(); };
  }, [vault]);




  const doSave = useCallback(async (forceOverwrite = false): Promise<SaveOutcome> => {
    // Serialize saves: wait for an in-flight write, then re-evaluate the buffer.
    if (savePromise.current) {
      await savePromise.current.catch(() => {});
      return doSave(forceOverwrite);
    }
    const doc = baseRef.current;
    const content = bufferRef.current;
    if (!doc) return 'clean';
    if (!forceOverwrite && content === doc.content) {
      setSaveState('saved');
      return 'clean';
    }
    const task = (async (): Promise<SaveOutcome> => {
      setSaveState('saving');
      setError('');
      try {
        const res = await api.writeNote(doc.path, content, doc.contentHash, doc.mtimeNs, doc.lineEnding, forceOverwrite);
        if (res.conflict) {
          setSaveState('conflict');
          setConflictMsg(res.message || 'File changed on disk.');
          setStatus(`Conflict: ${doc.path}`);
          return 'conflict';
        }
        if (res.saved && res.document) {
          setBaseDoc(res.document);
          // Keep buffer as-is if the user kept typing while the save was in flight.
          if (bufferRef.current === content) setSaveState('saved');
          else setSaveState('dirty');
          setConflictMsg('');
          setStatus(`Saved ${doc.path}`);
          return 'saved';
        }
        return 'clean';
      } catch (e) {
        setSaveState('error');
        setError(e instanceof Error ? e.message : String(e));
        return 'error';
      }
    })();
    savePromise.current = task;
    try {
      return await task;
    } finally {
      if (savePromise.current === task) savePromise.current = null;
    }
  }, []);

  const scheduleSave = useCallback(() => {
    if (saveTimer.current) window.clearTimeout(saveTimer.current);
    const ms = Math.max(300, settings?.autosaveMs ?? 750);
    saveTimer.current = window.setTimeout(() => {
      saveTimer.current = null;
      void doSave(false);
    }, ms);
  }, [doSave, settings?.autosaveMs]);

  // Flush pending edits before navigating away. Returns false when the write
  // failed or needs conflict resolution, so callers can avoid losing the buffer.
  const flushSave = useCallback(async (): Promise<boolean> => {
    if (saveTimer.current) {
      window.clearTimeout(saveTimer.current);
      saveTimer.current = null;
    }
    const outcome = await doSave(false);
    return outcome === 'clean' || outcome === 'saved';
  }, [doSave]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
        e.preventDefault();
        if (saveTimer.current) {
          window.clearTimeout(saveTimer.current);
          saveTimer.current = null;
        }
        void doSave(false);
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'o') {
        e.preventDefault();
        void chooseVault();
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
        e.preventDefault();
        (document.querySelector('.search') as HTMLInputElement | null)?.focus();
      }
      if (e.key === 'Escape') {
        setConflictMsg('');
        if (saveState === 'conflict') setSaveState(dirty ? 'dirty' : 'idle');
        if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [doSave, dirty, saveState]);


  async function chooseVault() {
    setError('');
    try {
      const picked = await api.openVaultDialog();
      if (!picked) return;
      await openPath(picked);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  async function openPath(path: string) {
    // Do not abandon unsaved/conflicted edits when switching vaults.
    if (!(await flushSave())) return;
    setIndexBusy(true);
    try {
      const info = await api.openVault(path);
      setVault(info);
      setBaseDoc(null);
      setBuffer('');
      setQuery('');
      setResults([]);
      setTree(await api.getTree());
      setSettings(await api.getSettings());
      setStatus(`Opened ${info.name}`);
      setError('');
      setSaveState('idle');
    } finally {
      setIndexBusy(false);
    }
  }

  async function selectNode(n: TreeNode) {
    if (n.isDir) return;
    if (baseDoc && bufferRef.current !== baseDoc.content) {
      // Abort navigation when the current note cannot be saved safely.
      if (!(await flushSave())) return;
    }
    setError('');
    try {
      const d = await api.readNote(n.path);
      setBaseDoc(d);
      setBuffer(d.content);
      setSaveState('idle');
      setConflictMsg('');
      setStatus(`Opened ${n.path}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  async function reloadFromDisk() {
    if (!baseDoc) return;
    try {
      const d = await api.readNote(baseDoc.path);
      setBaseDoc(d);
      setBuffer(d.content);
      setSaveState('idle');
      setConflictMsg('');
      setStatus(`Reloaded ${d.path}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  async function keepMyVersion() {
    await doSave(true);
  }

  async function saveCopy() {
    if (!baseDoc) return;
    try {
      const res = await api.saveConflictCopy(baseDoc.path, buffer, baseDoc.lineEnding);
      setConflictMsg(res.message || 'Saved conflict copy.');
      setStatus(res.document?.path ? `Saved copy: ${res.document.path}` : 'Saved conflict copy');
      setTree(await api.getTree());
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  async function openResult(path: string) {
    await selectNode({ name: path, path, absPath: '', isDir: false } as TreeNode);
  }

  async function rebuildIndex() {
    if (!vault) return;
    setIndexBusy(true);
    setError('');
    try {
      await api.rebuildIndex();
      setStatus(`Index rebuilt for ${vault.name}`);
      if (query.trim()) setResults(await api.search(query.trim(), { limit: 50 }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIndexBusy(false);
    }
  }

  function onEditorChange(next: string) {
    setBuffer(next);
    setSaveState('dirty');
    scheduleSave();
  }

  const saveLabel = saveState === 'saving' ? 'Saving…' : saveState === 'saved' ? 'Saved' : saveState === 'conflict' ? 'Conflict' : dirty ? 'Unsaved' : 'Ready';

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">Markvault</div>
        <div className="actions">
          <button onClick={chooseVault}>Open Vault</button>
          {vault ? <button onClick={rebuildIndex} disabled={indexBusy}>{indexBusy ? 'Indexing…' : 'Rebuild Index'}</button> : null}
          {baseDoc ? <button onClick={() => setPreview((p) => !p)}>{preview ? 'Edit' : 'Preview'}</button> : null}
          {baseDoc ? <button onClick={() => void doSave(false)} disabled={!dirty || saveState === 'saving'}>Save</button> : null}
        </div>
        <div className="meta">{vault ? `${vault.name} · ${fileCount} notes` : 'Local-first Markdown notes'}</div>
      </header>
      {error ? <div className="error">{error}</div> : null}
      {saveState === 'conflict' ? (
        <div className="conflict">
          <div className="conflict-text">{conflictMsg || 'This file changed on disk while you had edits.'}</div>
          <div className="conflict-actions">
            <button onClick={() => void reloadFromDisk()}>Reload from disk</button>
            <button onClick={() => void keepMyVersion()}>Keep my version</button>
            <button onClick={() => void saveCopy()}>Save as copy</button>
          </div>
        </div>
      ) : null}
      <div className="layout">
        <aside className="sidebar">
          {vault ? (
            <>
              <div className="panel-title">Search</div>
              <input
                className="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search notes (FTS5, Ctrl+F)"
                spellCheck={false}
              />
              {query.trim() ? (
                <div className="search-results">
                  <div className="muted small">{searching ? 'Searching…' : `${results.length} result${results.length === 1 ? '' : 's'}`}</div>
                <ul className="results">
                  {results.map((r) => (
                    <li key={r.path}>
                      <button className="result" onClick={() => void openResult(r.path)}>
                        <div className="result-title">{r.title || r.path}</div>
                        <div className="result-path">{r.path}</div>
                        <div className="result-snippet" dangerouslySetInnerHTML={{ __html: renderSnippet(r.snippet) }} />
                      </button>
                    </li>
                  ))}
                </ul>

                </div>
              ) : null}
              <div className="panel-title">Files</div>
              <div className="row">
                <button onClick={async () => {
                  if (!vault) return;
                  const name = window.prompt('New note path (e.g. Notes/Idea.md):');
                  if (!name?.trim()) return;
                  const target = withMarkdownExt(name);
                  if (!(await flushSave())) return;
                  try {
                    const title = target.replace(/\.md$/i, '').split('/').pop() || target;
                    const doc = await api.createNote(target, '# ' + title + '\n');
                    setTree(await api.getTree());
                    await selectNode({ name: target.split('/').pop() || target, path: doc.path, absPath: doc.absPath, isDir: false } as TreeNode);
                  } catch (e) { setError(e instanceof Error ? e.message : String(e)); }
                }}>New Note</button>
                <button onClick={async () => {
                  if (!vault) return;
                  const name = window.prompt('New folder path:');
                  if (!name) return;
                  try { await api.createFolder(name); setTree(await api.getTree()); } catch (e) { setError(e instanceof Error ? e.message : String(e)); }
                }}>New Folder</button>
              </div>
              <div className="tree-wrap"><TreeView nodes={tree} selected={baseDoc?.path} onSelect={(n) => void selectNode(n)} /></div>

            </>
          ) : (
            <>
              <div className="panel-title">Vault</div>
              <p className="muted">Open a folder containing Markdown files. Files are canonical; the SQLite index/cache lives outside the vault.</p>
              <button onClick={chooseVault}>Choose folder… (Ctrl+O)</button>
              {settings?.recentVaults?.length ? (
                <>
                  <div className="panel-title">Recent</div>
                  <ul className="recent">
                    {settings.recentVaults.map((p: string) => (
                      <li key={p}><button className="link" onClick={() => void openPath(p).catch((e) => setError(String(e)))} title={p}>{p}</button></li>
                    ))}
                  </ul>
                </>
              ) : null}
            </>
          )}
        </aside>
        <main className="main">
          {baseDoc ? (
            <>
              <div className="doc-header">
                <div className="doc-title">{baseDoc.path}</div>
                <div className="doc-actions">
                  <button onClick={async () => {
                    const next = window.prompt('Rename/move path (.md added if omitted):', baseDoc.path);
                    if (!next?.trim()) return;
                    const target = withMarkdownExt(next);
                    if (target === baseDoc.path) return;
                    if (!(await flushSave())) return;
                    try {
                      await api.renamePath(baseDoc.path, target);
                      const d = await api.readNote(target);
                      setBaseDoc(d); setBuffer(d.content); setTree(await api.getTree()); setStatus(`Renamed to ${d.path}`);
                    } catch (e) { setError(e instanceof Error ? e.message : String(e)); }
                  }}>Rename</button>
                  <button className="danger" onClick={async () => {
                    const target = baseDoc.path;
                    if (!window.confirm(`Move "${target}" to OS trash? (No permanent delete fallback)`)) return;
                    // Flush/clear any pending autosave so it cannot recreate the
                    // file after it has been moved to the trash.
                    if (saveTimer.current) {
                      window.clearTimeout(saveTimer.current);
                      saveTimer.current = null;
                    }
                    await doSave(false).catch(() => {});
                    try {
                      await api.deletePath(target);
                      setBaseDoc(null); setBuffer(''); setSaveState('idle'); setConflictMsg(''); setTree(await api.getTree()); setStatus(`Moved to trash: ${target}`);
                    } catch (e) { setError(e instanceof Error ? e.message : String(e)); }
                  }}>Delete</button>

                </div>
                <div className={`doc-meta save-${saveState}`}>{saveLabel} · {baseDoc.lineEnding === '\r\n' ? 'CRLF' : 'LF'}</div>
              </div>

              <div className="editor-wrap">
                {preview ? <MarkdownPreview content={buffer} /> : <MarkdownEditor value={buffer} path={baseDoc.path} onChange={onEditorChange} onSave={() => void doSave(false)} />}
              </div>
              <div className="muted footnote">Markdown files are canonical. Autosave is debounced; external changes require conflict resolution (Decision 8). Preview uses GFM with raw HTML disabled (Decision 13).</div>
            </>
          ) : (
            <div className="empty">
              <h2>{vault ? 'Select a note' : 'Welcome to Markvault'}</h2>
              <p className="muted">{vault ? 'Choose a Markdown file or use search.' : 'A distraction-free, local-first notes app for your Markdown folders.'}</p>
            </div>
          )}
        </main>
      </div>
      <footer className="statusbar">{status}{vault ? ` · ${vault.rootPath}` : ''}</footer>
    </div>
  );
}

export default App;
