/**
 * Shipped default design: the favourite design code chosen in the designer
 * trial (DOCKLET-DESIGN-1, decoded). Plain data without imports, so every
 * store derives its defaults from here through its own merge function and no
 * store's defaults depend on another store. Saved browser preferences still
 * win; "Reset to Docklet default" applies exactly this snapshot. Holds no
 * hosts, host order, service addresses or other operating data.
 */
export const SHIPPED_DESIGN = {
  tokens: { theme: "dark", palette: { primary: "#9333ea", secondary: "#00ffcc", surface: "#13111c" }, semantic: 1.12, glow: 1, motion: "playful", speed: 1, density: "compact", radius: 0.8, fontScale: 1 },
  uiStyle: {
    toggleStyle: "minimal",
    metricDisplay: "bars",
    hostFilterStyle: "menu",
    pointerGlow: true,
    pointerGlowIntensity: 0.6,
    toastScale: 1.6,
    toastDesign: "bar",
    actionDialogDesign: "focus",
    actionPresentationOverride: "hub",
    runMotion: "sweep",
  },
  visual: {
    logoVariant: "dropcap",
    logoMotion: "relay",
    logoEntrance: true,
    glowInside: 50,
    glowOutside: 4,
    glowSize: 128,
    glowLine: 2,
    dotWave: true,
    dotWaveStrength: 0.9,
    dotWavePeriod: 32,
    sidebarWidth: 17.5,
  },
  dashboard: {
    order: ["attention", "updates", "load", "security", "capacity", "events", "timeline", "overseerr", "plex", "schedule", "minecraft", "apps"],
    hidden: ["minecraft", "load", "timeline", "capacity", "updates"],
    sizes: {
      attention: { w: 1, h: 6 },
      updates: { w: 1, h: 6 },
      timeline: { w: 2, h: 4 },
      security: { w: 2, h: 3 },
      load: { w: 2, h: 3 },
      apps: { w: 2, h: 3 },
      overseerr: { w: 1, h: 6 },
      events: { w: 1, h: 6 },
      schedule: { w: 1, h: 6 },
      plex: { w: 1, h: 6 },
    },
  },
  fleet: { mode: "joint", ringSize: 320, placement: "right", fan: true, hostArea: { w: 5, h: 2 } },
  help: { mode: "full" },
  overview: { stacksMode: "compact", containersMode: "compact", stacksLayout: "hosts", containersLayout: "hosts", volumesLayout: "table", imagesLayout: "table", workspaceAside: "details" },
} as const;
