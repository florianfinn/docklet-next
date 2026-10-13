// German interface texts follow the English base. Feature files own their texts.
import { deActivity } from "@/features/activity/messages/de";
import { deLogin } from "@/features/auth/login/messages/de";
import { deContainers } from "@/features/containers/messages/de";
import { deDashboard } from "@/features/dashboard/messages/de";
import { deHosts } from "@/features/hosts/messages/de";
import { deImages } from "@/features/images/messages/de";
import { deSettings } from "@/features/settings/messages/de";
import { deSetup } from "@/features/setup/messages/de";
import { deVolumes } from "@/features/volumes/messages/de";
import { deLab } from "@/shared/preferences/messages/de";
import { deVisual } from "@/shared/preferences/messages/visual-de";
import { deNetworks } from "@/features/networks/messages/de";
import { dePoolErrors } from "@/features/networks/messages/pool-de";
import { deSecurity } from "@/features/security/messages/de";
import { deEditor } from "@/features/stacks/editor/messages/de";
import { deStacks } from "@/features/stacks/messages/de";
import { deHelp } from "@/shared/help/messages/de";
import { deCommon } from "@/shared/messages/de";
import { deTheme } from "@/shared/theme/messages/de";
import { deWorkspace } from "@/shared/workspace/messages/de";

import { dePreview } from "../preview/messages/de";
import { deShell } from "../shell/messages/de";
import { deLocale } from "./messages/de";

import type { enMessages } from "./messages-en";

export const messages: typeof enMessages = {
  common: deCommon,
  help: deHelp,
  workspace: deWorkspace,
  theme: deTheme,
  shell: deShell,
  preview: dePreview,
  activity: deActivity,
  lab: deLab,
  visual: deVisual,
  dashboard: deDashboard,
  stacks: deStacks,
  editor: deEditor,
  containers: deContainers,
  security: deSecurity,
  networks: {
    ...deNetworks,
    connectDialog: {
      ...deNetworks.connectDialog,
      ipErrors: { ...deNetworks.connectDialog.ipErrors, ...dePoolErrors.ipErrors },
      ip6Errors: { ...deNetworks.connectDialog.ip6Errors, ...dePoolErrors.ip6Errors },
    },
  },
  hosts: deHosts,
  images: deImages,
  volumes: deVolumes,
  settings: deSettings,
  setup: deSetup,
  login: deLogin,
  locale: deLocale,
};
