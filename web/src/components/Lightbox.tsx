import { useMemo } from "react";
import ReactLightbox from "yet-another-react-lightbox";
import Captions from "yet-another-react-lightbox/plugins/captions";
import Zoom from "yet-another-react-lightbox/plugins/zoom";
import "yet-another-react-lightbox/styles.css";
import "yet-another-react-lightbox/plugins/captions.css";
import { previewUrl } from "../api/client";
import { CHUNK_SIZE } from "../hooks/useSamples";
import type { Sample } from "../types";

interface Props {
  chunks: Map<number, Sample[]>;
  count: number;
  index: number;
  onClose: () => void;
  onNavigate: (index: number) => void;
}

// Operates only over currently loaded chunks (near wherever the grid was scrolled) rather than
// the whole dataset — consistent with never accumulating all samples client-side. Un-loaded
// positions render as a blank placeholder slide; scrolling the grid there first loads them.
export default function Lightbox({ chunks, count, index, onClose, onNavigate }: Props) {
  // ponytail: builds one placeholder object per sample in the dataset, since the library wants
  // a full slides array up front. Fine into the low millions; a true billion-scale lightbox
  // needs the library's lazy/windowed slide API, add when this array's size is measurably
  // slow to build.
  const slides = useMemo(() => {
    return Array.from({ length: count }, (_, i) => {
      const chunkOffset = Math.floor(i / CHUNK_SIZE) * CHUNK_SIZE;
      const sample = chunks.get(chunkOffset)?.[i - chunkOffset];
      if (!sample) return { src: "", width: 1, height: 1, alt: "not loaded" };
      return {
        src: previewUrl(sample.id),
        width: sample.width,
        height: sample.height,
        alt: sample.filename,
        title: `${sample.filename} · ${i + 1} / ${count}`,
        description: sample.tags.length > 0 ? sample.tags.join(", ") : undefined,
      };
    });
  }, [chunks, count]);

  return (
    <ReactLightbox
      open
      close={onClose}
      index={index}
      on={{ view: ({ index: i }) => onNavigate(i) }}
      slides={slides}
      plugins={[Zoom, Captions]}
      zoom={{ maxZoomPixelRatio: 4 }}
    />
  );
}
