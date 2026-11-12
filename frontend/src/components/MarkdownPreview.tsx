import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Components } from 'react-markdown';

function safeHref(href?: string): string | undefined {
  if (!href) return undefined;
  const s = href.trim();
  if (!s) return undefined;
  const lower = s.toLowerCase();
  // Decision 13: neutralize dangerous URL schemes. Local vault images/paths deferred.
  if (lower.startsWith('javascript:') || lower.startsWith('vbscript:')) return undefined;
  if (lower.startsWith('data:')) {
    if (lower.startsWith('data:image/png;') || lower.startsWith('data:image/jpeg;') || lower.startsWith('data:image/gif;') || lower.startsWith('data:image/webp;')) return s;
    return undefined;
  }
  return s;
}

function isRemoteImage(src: string): boolean {
  const s = src.trim();
  return /^(https?:)?\/\//i.test(s) || /^data:image\/(?:png|jpe?g|gif|webp);/i.test(s);
}

const components: Components = {
  a: ({ href, children, ...props }) => {
    const safe = safeHref(href);
    if (!safe) return <span className="unsafe-link">{children}</span>;
    return <a {...props} href={safe} target="_blank" rel="noreferrer noopener">{children}</a>;
  },
  img: ({ src, alt, ...props }) => {
    // Decision 13: local vault image rendering deferred; render non-dangerous remote/data-image only.
    const raw = typeof src === 'string' ? src : undefined;
    const safe = safeHref(raw);
    if (!safe || !isRemoteImage(safe)) {
      return <span className="deferred-image">[image: {alt || safe || 'local image rendering deferred'}]</span>;
    }
    return <img {...props} src={safe} alt={alt || ''} loading="lazy" />;
  },
};

export function MarkdownPreview({ content }: { content: string }) {
  return (
    <div className="preview markdown-body">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        components={components}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
