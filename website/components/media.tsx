import { asset } from "@/lib/site";

type MediaProps = {
  src: string;
  alt: string;
  caption?: string;
  className?: string;
  priority?: boolean;
  /** Plots and diagrams on a light background get a white card. */
  light?: boolean;
};

/**
 * A GIF, screenshot or chart from public/media. The terminal recordings are
 * rendered from real interpreter runs by `npm run media`.
 */
export function Media({
  src,
  alt,
  caption,
  className = "",
  priority,
  light,
}: MediaProps) {
  const img = (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={asset(src.startsWith("/") ? src : `/media/${src}`)}
      alt={alt}
      loading={priority ? "eager" : "lazy"}
      className={`w-full rounded-2xl shadow-2xl ring-1 shadow-black/50 ring-white/10 ${
        light ? "bg-white p-3" : ""
      } ${className}`}
    />
  );
  if (!caption) return img;
  return (
    <figure className="not-prose my-8">
      {img}
      <figcaption className="mt-3 text-center text-sm text-mist-400">
        {caption}
      </figcaption>
    </figure>
  );
}
