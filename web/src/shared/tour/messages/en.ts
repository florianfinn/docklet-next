// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour texts (English base).

// American English texts of the guided tour offered after first-run setup.
// Step keys match TOUR_STEPS in ../tour-steps.ts.

export const enTour = {
  offer: {
    title: "Setup complete",
    text: "Take a short tour of the most important pages? It takes about a minute and changes nothing.",
    start: "Start tour",
    later: "Not now",
    hint: "You can start the tour at any time in Settings › Explanations.",
  },
  progress: "Step {current} of {total}",
  back: "Back",
  next: "Next",
  finish: "Finish",
  skip: "End tour",
  openNav: "Open “{page}” in the navigation on the left, or select Next.",
  openMenu: "Open the menu at the top left, then “{page}”, or select Next.",
  label: "Guided tour",
  steps: {
    dashboard: {
      title: "Dashboard",
      text: "The situation of all hosts at a glance: the ring shows the stacks and standalone containers of the selected hosts by status, the panel next to it lists what needs attention first. Customize arranges the widgets.",
    },
    hostFilter: {
      title: "Host filter",
      text: "Choose which hosts the pages show. The filter applies to the dashboard, stacks, containers and resources alike.",
    },
    hosts: {
      title: "Hosts",
      text: "Connect further Docker hosts here, directly or via Edge for hosts behind NAT. Each host shows its status, agent version and maintenance.",
    },
    stacks: {
      title: "Stacks",
      text: "Compose projects: create a stack, edit the Compose file and environment, deploy it and keep images up to date.",
    },
    containers: {
      title: "Containers",
      text: "All containers of the selected hosts with status and load. Open a container for logs, terminal, files and settings.",
    },
    security: {
      title: "Security",
      text: "Image scan results by severity. Each finding shows the affected containers and whether a fixed version exists.",
    },
    backups: {
      title: "Backups",
      text: "Back up volumes on a schedule and restore them. The hub database has its own backups under Hub database.",
    },
    notifications: {
      title: "Notifications",
      text: "Get notified about failures, updates and findings, for example by email, ntfy, Telegram or Discord.",
    },
    search: {
      title: "Search and activity",
      text: "Search finds pages, resources and quick actions (Ctrl+K). Activity shows running and finished actions. Restart this tour any time in Settings › Explanations.",
    },
  },
};
