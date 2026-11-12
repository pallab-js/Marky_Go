import { markdown } from '@codemirror/lang-markdown';
import { EditorState } from '@codemirror/state';
import { EditorView, keymap, placeholder } from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { oneDark } from '@codemirror/theme-one-dark';
import { useEffect, useRef } from 'react';

export function MarkdownEditor({
  value,
  path,
  onChange,
  onSave,
}: {
  value: string;
  path: string;
  onChange: (next: string) => void;
  onSave: () => void;
}) {
  const host = useRef<HTMLDivElement | null>(null);
  const view = useRef<EditorView | null>(null);
  const syncing = useRef(false);
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
  onChangeRef.current = onChange;
  onSaveRef.current = onSave;

  useEffect(() => {
    if (!host.current) return;
    const state = EditorState.create({
      doc: value,
      extensions: [
        markdown(),
        history(),
        oneDark,
        keymap.of([
          { key: 'Mod-s', run: () => { onSaveRef.current(); return true; } },
          indentWithTab,
          ...defaultKeymap,
          ...historyKeymap,
        ]),
        EditorView.updateListener.of((u) => {
          if (u.docChanged && !syncing.current) onChangeRef.current(u.state.doc.toString());
        }),
        EditorView.lineWrapping,
        placeholder('Start writing Markdown…'),
        EditorView.theme({
          '&': { height: '100%', fontSize: '14px' },
          '.cm-scroller': { overflow: 'auto', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' },
          '.cm-content': { padding: '16px 0' },
          '.cm-line': { padding: '0 16px' },
        }),
      ],
    });
    const v = new EditorView({ state, parent: host.current });
    view.current = v;
    v.focus();
    return () => {
      v.destroy();
      view.current = null;
    };
  }, [path]); // recreate per note path to keep history/doc isolated

  useEffect(() => {
    const v = view.current;
    if (!v) return;
    const current = v.state.doc.toString();
    if (current !== value) {
      // Programmatic resync (external reload / note switch) must not be
      // reported as a user edit, or it would schedule a phantom autosave.
      syncing.current = true;
      try {
        v.dispatch({ changes: { from: 0, to: current.length, insert: value } });
      } finally {
        syncing.current = false;
      }
    }
  }, [value]);

  return <div className="cm-host" ref={host} />;
}
