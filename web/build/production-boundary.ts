import type { Plugin } from "vite";

const forbiddenText = /Design-Labor|Fehlersimulation|React-Prototyp|React prototype|design lab|Demo artifact|DEMO ARTIFACT|Demo-Daten|Demo data|\(Demo\)|\(demo\)|Alex Beispiel|alex@example\.com|Demo ·/;

function checkAsset(fileName: string, content: string): void {
  if (/\.(?:js|css|html)$/.test(fileName) && forbiddenText.test(content)) {
    throw new Error(`Production boundary: forbidden text in ${fileName}`);
  }
}

/** Check the complete graph, including modules later removed by tree shaking. */
export function productionBoundary(): Plugin {
  return {
    name: "docklet-production-boundary",
    apply: "build",
    generateBundle(_options, bundle) {
      const modules = [...this.getModuleIds()].map((id) => id.replaceAll("\\", "/"));
      const forbidden = modules.filter((id) => /\/src\/(?:data\/demo\/|features\/design-lab\/|[^/]*\.dev\.|features\/auth\/login\/login-model\.|features\/activity\/(?:NotificationLab|sample-run)\.|test\/|[^/]*\.(?:test|test-support)\.)/.test(id));
      if (forbidden.length) {
        this.error(`Production boundary: forbidden module ${forbidden[0].split("/src/").at(-1)}`);
      }
      const shipped = new Set<string>();
      for (const output of Object.values(bundle)) {
        if (output.type === "chunk") {
          for (const id of Object.keys(output.modules)) shipped.add(id.replaceAll("\\", "/"));
          checkAsset(output.fileName, output.code);
        } else if (typeof output.source === "string") {
          checkAsset(output.fileName, output.source);
        }
      }
      const demoCatalog = [...shipped].filter((id) => /\/src\/(?:shared\/preferences\/messages\/(?:de|en)|features\/auth\/login\/messages\/(?:de|en)|app\/preview\/messages\/(?:de|en)|features\/setup\/messages\/(?:de|en))\.ts$/.test(id));
      if (demoCatalog.length) this.error(`Production boundary: shipped demo catalogue ${demoCatalog[0].split("/src/").at(-1)}`);
      const sourcePaths = (ids: Iterable<string>) => [...ids].filter((id) => id.includes("/src/") && !id.includes("/node_modules/")).map((id) => `src/${id.split("/src/").at(-1)}`).sort();
      this.emitFile({
        type: "asset",
        fileName: "production-modules.json",
        source: JSON.stringify({ graph: sourcePaths(modules), shipped: sourcePaths(shipped) }, null, 2),
      });
    },
    transformIndexHtml: {
      order: "post",
      handler(html) {
        checkAsset("index.html", html);
        return html;
      },
    },
  };
}
