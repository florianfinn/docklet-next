import type { Plugin } from "vite";

/** Check the complete graph, including modules later removed by tree shaking. */
export function productionBoundary(): Plugin {
  return {
    name: "docklet-production-boundary",
    apply: "build",
    generateBundle(_options, bundle) {
      const modules = [...this.getModuleIds()].map((id) => id.replaceAll("\\", "/"));
      const forbidden = modules.filter((id) => /\/src\/(?:data\/demo\/|features\/design-lab\/|runtime\.dev\.|features\/auth\/login\/login-model\.|features\/activity\/(?:NotificationLab|sample-run)\.)/.test(id));
      if (forbidden.length) {
        this.error(`Production boundary: forbidden module ${forbidden[0].split("/src/").at(-1)}`);
      }
      const shipped = new Set<string>();
      for (const output of Object.values(bundle)) {
        if (output.type === "chunk") for (const id of Object.keys(output.modules)) shipped.add(id.replaceAll("\\", "/"));
      }
      const sourcePaths = (ids: Iterable<string>) => [...ids].filter((id) => id.includes("/src/") && !id.includes("/node_modules/")).map((id) => `src/${id.split("/src/").at(-1)}`).sort();
      this.emitFile({
        type: "asset",
        fileName: "production-modules.json",
        source: JSON.stringify({ graph: sourcePaths(modules), shipped: sourcePaths(shipped) }, null, 2),
      });
    },
  };
}
