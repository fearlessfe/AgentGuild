// Render the AgentGuild full-flow gallery to PNGs and a multi-page PDF using
// the repository's Playwright Chromium (installed under frontend/).
//
//   node design/render-mockups.mjs
//
// Canonical viewport is 1440×1024 at device scale factor 2. The ten flow
// screens render in both dark and light themes; boards render once.

import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { dirname, join, extname } from "node:path";
import { mkdir, readFile } from "node:fs/promises";
import { createServer } from "node:http";

const require = createRequire(join(process.cwd(), "frontend", "package.json"));
const { chromium } = require("playwright");

const root = dirname(fileURLToPath(import.meta.url));
const galleryRoot = join(root, "agentguild-full-flow");
const outDir = join(root, "..", "docs", "assets", "full-flow");

const VIEWPORT = { width: 1440, height: 1024 };
const SCALE = 2;

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
};

// A minimal static server so ES module imports resolve over http:// —
// browsers block cross-file module fetches over file://.
function startServer() {
  return new Promise((resolve) => {
    const server = createServer(async (req, res) => {
      const path = decodeURIComponent((req.url || "/").split("?")[0]);
      const file = join(galleryRoot, path === "/" ? "index.html" : path);
      try {
        const body = await readFile(file);
        res.writeHead(200, { "content-type": MIME[extname(file)] || "application/octet-stream" });
        res.end(body);
      } catch {
        res.writeHead(404);
        res.end("not found");
      }
    });
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      resolve({ server, base: `http://127.0.0.1:${port}` });
    });
  });
}

// board id -> output filename (rendered once, dark theme)
const BOARDS = [
  ["journey", "00-full-flow-journey"],
  ["api-coverage", "00-api-coverage"],
  ["components-states", "11-components-and-states"],
  ["responsive-rules", "12-responsive-rules"],
];

// flow screen id -> filename prefix (rendered in dark + light)
const FLOW = [
  ["login", "01-login"],
  ["onboarding", "02-onboarding"],
  ["git-integration", "03-git-integration"],
  ["repository-sync-rule", "04-repository-sync-rule"],
  ["sync-result", "05-sync-result"],
  ["task-center", "06-task-center"],
  ["execution-detail", "07-execution-detail"],
  ["submission-validation", "08-submission-validation"],
  ["review-workspace", "09-review-workspace"],
  ["outcome", "10-outcome"],
];

async function shoot(page, base, screen, theme, file) {
  const url = `${base}/index.html?screen=${screen}&theme=${theme}`;
  await page.goto(url, { waitUntil: "networkidle" });
  await page.evaluate(() => document.fonts.ready);
  const node = await page.waitForSelector(".screen");
  await node.screenshot({ path: join(outDir, `${file}.png`) });
  process.stdout.write(`  ✓ ${file}.png\n`);
}

async function main() {
  await mkdir(outDir, { recursive: true });
  const { server, base } = await startServer();
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: SCALE,
  });
  const page = await context.newPage();

  console.log("Boards:");
  for (const [screen, file] of BOARDS) {
    await shoot(page, base, screen, "dark", file);
  }

  console.log("Flow screens (dark + light):");
  for (const [screen, prefix] of FLOW) {
    await shoot(page, base, screen, "dark", `${prefix}-dark`);
    await shoot(page, base, screen, "light", `${prefix}-light`);
  }

  console.log("PDF:");
  const pdfPage = await context.newPage();
  await pdfPage.goto(`${base}/index.html?print=all&theme=dark`, {
    waitUntil: "networkidle",
  });
  await pdfPage.evaluate(() => document.fonts.ready);
  await pdfPage.emulateMedia({ media: "print" });
  await pdfPage.pdf({
    path: join(outDir, "agentguild-full-flow.pdf"),
    width: "1440px",
    height: "1024px",
    printBackground: true,
  });
  process.stdout.write("  ✓ agentguild-full-flow.pdf\n");

  await browser.close();
  server.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
