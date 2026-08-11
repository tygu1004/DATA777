import { Assets, type Texture } from "pixi.js";

// Bounds resident texture memory (architecture.md#frontend: "resident textures must be capped
// rather than grown freely"). ~136x136 RGBA is ~74KB; 600 textures is ~44MB, a reasonable cap.
const MAX_TEXTURES = 600;
const lruOrder: string[] = [];

export async function getTexture(url: string): Promise<Texture> {
  // /api/thumbnails/{id} has no file extension, so Pixi's asset resolver can't infer a parser
  // from the URL alone — `format` tells it explicitly this is a jpeg to decode as a texture.
  const texture = await Assets.load<Texture>({ src: url, format: "jpg", parser: "loadTextures" });
  touch(url);
  return texture;
}

function touch(url: string) {
  const i = lruOrder.indexOf(url);
  if (i !== -1) lruOrder.splice(i, 1);
  lruOrder.push(url);
  while (lruOrder.length > MAX_TEXTURES) {
    const evict = lruOrder.shift();
    if (evict) Assets.unload(evict).catch(() => {});
  }
}
