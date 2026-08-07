import ReactLightbox from "yet-another-react-lightbox";
import Captions from "yet-another-react-lightbox/plugins/captions";
import Zoom from "yet-another-react-lightbox/plugins/zoom";
import "yet-another-react-lightbox/styles.css";
import "yet-another-react-lightbox/plugins/captions.css";
import { previewUrl } from "../api/client";
import type { Sample } from "../types";

interface Props {
  samples: Sample[];
  index: number;
  onClose: () => void;
  onNavigate: (index: number) => void;
}

export default function Lightbox({ samples, index, onClose, onNavigate }: Props) {
  return (
    <ReactLightbox
      open
      close={onClose}
      index={index}
      on={{ view: ({ index: i }) => onNavigate(i) }}
      slides={samples.map((s, i) => ({
        src: previewUrl(s.id),
        width: s.width,
        height: s.height,
        alt: s.filename,
        title: `${s.filename} · ${i + 1} / ${samples.length}`,
        description: s.tags.length > 0 ? s.tags.join(", ") : undefined,
      }))}
      plugins={[Zoom, Captions]}
      zoom={{ maxZoomPixelRatio: 4 }}
    />
  );
}
