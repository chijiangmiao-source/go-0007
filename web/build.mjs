import { mkdir, copyFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const dist = join(root, "dist");
await mkdir(dist, { recursive: true });
for (const file of ["index.html", "app.js", "styles.css"]) {
  await copyFile(join(root, "src", file), join(dist, file));
}
