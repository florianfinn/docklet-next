// American English interface texts define the catalogue shape for every locale.
import { enActivity } from "@/features/activity/messages/en";
import { enLogin } from "@/features/auth/login/messages/en";
import { enContainers } from "@/features/containers/messages/en";
import { enDashboard } from "@/features/dashboard/messages/en";
import { enHosts } from "@/features/hosts/messages/en";
import { enImages } from "@/features/images/messages/en";
import { enSettings } from "@/features/settings/messages/en";
import { enSetup } from "@/features/setup/messages/en";
import { enVolumes } from "@/features/volumes/messages/en";
import { enLab } from "@/shared/preferences/messages/en";
import { enVisual } from "@/shared/preferences/messages/visual-en";
import { enNetworks } from "@/features/networks/messages/en";
import { enPoolErrors } from "@/features/networks/messages/pool-en";
import { enSecurity } from "@/features/security/messages/en";
import { enEditor } from "@/features/stacks/editor/messages/en";
import { enStacks } from "@/features/stacks/messages/en";
import { enHelp } from "@/shared/help/messages/en";
import { enCommon } from "@/shared/messages/en";
import { enTheme } from "@/shared/theme/messages/en";
import { enWorkspace } from "@/shared/workspace/messages/en";

import { enPreview } from "../preview/messages/en";
import { enShell } from "../shell/messages/en";
import { enLocale } from "./messages/en";

export const enMessages = {
  common: enCommon,
  help: enHelp,
  workspace: enWorkspace,
  theme: enTheme,
  shell: enShell,
  preview: enPreview,
  activity: enActivity,
  lab: enLab,
  visual: enVisual,
  dashboard: enDashboard,
  stacks: enStacks,
  editor: enEditor,
  containers: enContainers,
  security: enSecurity,
  networks: {
    ...enNetworks,
    connectDialog: {
      ...enNetworks.connectDialog,
      ipErrors: { ...enNetworks.connectDialog.ipErrors, ...enPoolErrors.ipErrors },
      ip6Errors: { ...enNetworks.connectDialog.ip6Errors, ...enPoolErrors.ip6Errors },
    },
  },
  hosts: enHosts,
  images: enImages,
  volumes: enVolumes,
  settings: enSettings,
  setup: enSetup,
  login: enLogin,
  locale: enLocale,
};
