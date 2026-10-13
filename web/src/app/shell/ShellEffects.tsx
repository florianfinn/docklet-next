import { useEffect } from "react";
import { useLocation } from "react-router";

import { activityStore } from "@/features/activity/activity-store";
import { labOpenStore } from "@/features/design-lab/use-design-lab";
import { closeAllPopups } from "@/shared/lib/popup-coordinator";
import { useSidebar } from "@/shared/ui/sidebar";

/**
 * Shell-wide reactions to context changes, independent of the component set:
 * the mobile navigation sheet closes after every navigation, and info popups
 * close on route, tab or query changes and when a side sheet opens.
 */
export function ShellEffects() {
  const { pathname, search } = useLocation();
  const { setOpenMobile } = useSidebar();

  useEffect(() => {
    setOpenMobile(false);
  }, [pathname, setOpenMobile]);

  useEffect(() => {
    closeAllPopups();
  }, [pathname, search]);

  useEffect(() => {
    const unsubLab = labOpenStore.subscribe(() => labOpenStore.get() && closeAllPopups());
    const unsubActivity = activityStore.subscribe(() => activityStore.get().open && closeAllPopups());
    return () => {
      unsubLab();
      unsubActivity();
    };
  }, []);

  return null;
}
