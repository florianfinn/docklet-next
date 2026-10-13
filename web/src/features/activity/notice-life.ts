import { useEffect } from "react";

/**
 * Lifetime of small notices in ms; notifications.css mirrors these values as
 * --notice-life. Errors stay longer so their text can be read.
 */
export const NOTICE_LIFE = { default: 6000, error: 10000 } as const;

/**
 * The countdown bar is the notice's only timer, so bar and lifetime pause
 * together on hover, keyboard focus, Alt+T and while the page is hidden: when
 * the bar's animation ends, the notice closes through its own close button.
 */
export function useNoticeLifetime(): void {
  useEffect(() => {
    const root = document.documentElement;
    const onEnd = (e: AnimationEvent) => {
      if (e.animationName !== "notice-life" || !(e.target instanceof HTMLElement) || !e.target.matches("[data-sonner-toast]")) return;
      e.target.querySelector<HTMLButtonElement>("[data-close-button]")?.click();
    };
    const onVisibility = () => root.toggleAttribute("data-page-hidden", document.hidden);
    onVisibility();
    document.addEventListener("animationend", onEnd);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("animationend", onEnd);
      document.removeEventListener("visibilitychange", onVisibility);
      root.removeAttribute("data-page-hidden");
    };
  }, []);
}
