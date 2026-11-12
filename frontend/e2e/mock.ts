import type { Page } from '@playwright/test';

export const DEFAULT_FILES: Record<string, string> = {
  'Welcome.md':
    '# Welcome\n\nHello [world](https://example.com).\n\n<script>window.__pwned = 1</script>\n\n[bad](javascript:alert(1))\n\n![remote](https://example.com/x.png)\n\n![local](./img.png)\n',
  'Notes/Idea.md': '# Idea\n\nA searchable thought about kittens.\n',
  'Notes/sub/Deep.md': '# Deep\n\nNested content.\n',
};

export const DEFAULT_SETTINGS = {
  recentVaults: ['/mock/vault'],
  lastActiveVault: '/mock/vault',
  autosaveMs: 250,
  previewDefault: false,
  configVersion: 1,
};

export async function installMock(
  page: Page,
  files: Record<string, string> = DEFAULT_FILES,
  settings: typeof DEFAULT_SETTINGS = DEFAULT_SETTINGS,
) {
  await page.addInitScript(
    ({ initialFiles, settings }) => {
      const hash = (s: string) => {
        let h = 5381;
        for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0;
        return h.toString(16);
      };
      const listeners: Record<string, Array<(...a: any[]) => void>> = {};
      const state = {
        files: { ...initialFiles } as Record<string, string>,
        settings: { ...settings },
        writes: [] as Array<{ path: string; content: string }>,
        lastWriteArgs: null as any,
        searchDelay: 0,
        writeDelay: 0,
      };
      const normalize = (p: string) => p.replace(/^\/+/, '').replace(/\\/g, '/');
      const docFor = (path: string) => {
        const p = normalize(path);
        const content = state.files[p];
        if (content === undefined) throw new Error(`file not found: ${p}`);
        return {
          path: p,
          absPath: `/mock/vault/${p}`,
          content,
          mtimeNs: Date.now() * 1e6,
          sizeBytes: content.length,
          contentHash: hash(content),
          lineEnding: '\n',
        };
      };
      const emit = (event: string, payload: any) => {
        for (const cb of listeners[event] || []) cb(payload);
      };
      const buildTree = () => {
        type Node = { name: string; path: string; absPath: string; isDir: boolean; children?: Node[] };
        const root: Node[] = [];
        const dirs = new Map<string, Node>();
        const ensureDir = (segments: string[]): Node[] => {
          let list = root;
          let rel = '';
          for (const seg of segments) {
            rel = rel ? `${rel}/${seg}` : seg;
            let node = dirs.get(rel);
            if (!node) {
              node = { name: seg, path: rel, absPath: `/mock/vault/${rel}`, isDir: true, children: [] };
              dirs.set(rel, node);
              list.push(node);
            }
            list = node.children!;
          }
          return list;
        };
        for (const path of Object.keys(state.files).sort()) {
          const segs = path.split('/');
          const name = segs.pop()!;
          const list = ensureDir(segs);
          list.push({ name, path, absPath: `/mock/vault/${path}`, isDir: false });
        }
        const sort = (nodes: Node[]): Node[] => {
          nodes.sort((a, b) => (a.isDir === b.isDir ? a.name.localeCompare(b.name) : a.isDir ? -1 : 1));
          for (const n of nodes) if (n.children) sort(n.children);
          return nodes;
        };
        return sort(root);
      };
      const api: any = {
        AppInfo: async () => ({ name: 'Markvault', dbPath: '/tmp/db', configPath: '/tmp/config' }),
        GetSettings: async () => ({ ...state.settings }),
        UpdateSettings: async (s: any) => {
          state.settings = { ...s };
          return { ...state.settings };
        },
        Health: async () => 'ok',
        OpenVaultDialog: async () => '/mock/vault',
        OpenVault: async (p: string) => {
          state.settings.lastActiveVault = p;
          if (!state.settings.recentVaults.includes(p)) state.settings.recentVaults.unshift(p);
          return { rootPath: p, name: 'vault', lastOpenedAt: new Date().toISOString() };
        },
        GetTree: async () => buildTree(),
        ReadNote: async (p: string) => docFor(p),
        RebuildIndex: async () => undefined,
        Search: async (q: string, opts: any) => {
          if (state.searchDelay > 0) await new Promise((r) => setTimeout(r, state.searchDelay));
          const needle = q.trim().toLowerCase();
          if (!needle) return [];
          const out = [];
          for (const [path, content] of Object.entries(state.files)) {
            const idx = content.toLowerCase().indexOf(needle);
            if (idx >= 0) {
              const start = Math.max(0, idx - 40);
              const end = Math.min(content.length, idx + needle.length + 40);
              const snippet = `${content.slice(start, idx)}\u0001${content.slice(idx, idx + needle.length)}\u0002${content.slice(idx + needle.length, end)}`;
              const title = (content.match(/^#\s+(.+)$/m) || [, path])[1] as string;
              out.push({ path, title, mtimeNs: Date.now() * 1e6, snippet, rank: 0 });
            }
            if (out.length >= (opts?.limit || 50)) break;
          }
          return out;
        },
        WriteNote: async (p: string, content: string, baseHash: string, mtime: number, le: string, force: boolean) => {
          if (state.writeDelay > 0) await new Promise((r) => setTimeout(r, state.writeDelay));
          state.lastWriteArgs = { p, content, baseHash, mtime, le, force };
          const np = normalize(p);
          const existing = state.files[np];
          if (existing !== undefined && !force && baseHash && hash(existing) !== baseHash) {
            return { saved: false, conflict: true, conflictKind: 'external_change', message: 'File changed on disk since it was opened.' };
          }
          state.files[np] = content;
          state.writes.push({ path: np, content });
          return { saved: true, conflict: false, document: docFor(np) };
        },
        SaveConflictCopy: async (p: string, content: string) => {
          const np = normalize(p).replace(/\.md$/i, '');
          const copy = `${np} (conflict copy).md`;
          state.files[copy] = content;
          return { saved: true, conflict: false, document: docFor(copy), message: 'Saved conflict copy' };
        },
        CreateNote: async (p: string, content: string) => {
          const np = normalize(p);
          if (state.files[np] !== undefined) throw new Error('file already exists');
          state.files[np] = content;
          return docFor(np);
        },
        CreateFolder: async () => undefined,
        RenamePath: async (oldRel: string, newRel: string) => {
          const from = normalize(oldRel);
          const to = normalize(newRel);
          if (!/\.md$/i.test(to)) throw new Error('only .md files are supported');
          if (state.files[to] !== undefined) throw new Error('target already exists');
          if (state.files[from] === undefined) throw new Error(`file not found: ${from}`);
          state.files[to] = state.files[from];
          delete state.files[from];
        },
        DeletePath: async (p: string) => {
          const np = normalize(p);
          if (state.files[np] === undefined) throw new Error(`file not found: ${np}`);
          delete state.files[np];
        },
      };
      const runtime: any = {
        EventsOnMultiple: (event: string, cb: (...a: any[]) => void) => {
          (listeners[event] ||= []).push(cb);
          return () => {
            listeners[event] = (listeners[event] || []).filter((x) => x !== cb);
          };
        },
        EventsOff: () => {},
        EventsOffAll: () => {},
        EventsEmit: () => {},
        LogPrint: () => {}, LogTrace: () => {}, LogDebug: () => {}, LogInfo: () => {}, LogWarning: () => {}, LogError: () => {}, LogFatal: () => {},
        WindowSetTitle: () => {},
        WindowSetDarkTheme: () => {},
        WindowSetLightTheme: () => {},
      };
      (window as any).go = { app: { App: api } };
      (window as any).runtime = runtime;
      (window as any).__mock = {
        state,
        emit,
        hash,
        setFile: (path: string, content: string) => {
          const p = normalize(path);
          state.files[p] = content;
          emit('vault:changed', { path: `/mock/vault/${p}`, cause: 'fsnotify' });
        },
        removeFile: (path: string) => {
          const p = normalize(path);
          delete state.files[p];
          emit('vault:changed', { path: `/mock/vault/${p}`, cause: 'fsnotify' });
        },
        content: (path: string) => state.files[normalize(path)],
        listeners,
      };
    },
    { initialFiles: files, settings },
  );
}
