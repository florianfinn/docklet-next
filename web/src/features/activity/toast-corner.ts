/**
 * Fixed viewport corner of the toast column: 16 px from the top and right
 * (8 px on phones), inside the safe area. New toasts stack downwards; page
 * headers, their buttons and open side sheets never move the column.
 */
export const TOAST_OFFSET = {
  top: "calc(16px + env(safe-area-inset-top, 0px))",
  right: "calc(16px + env(safe-area-inset-right, 0px))",
} as const;

export const TOAST_MOBILE_OFFSET = {
  top: "calc(8px + env(safe-area-inset-top, 0px))",
  right: "calc(8px + env(safe-area-inset-right, 0px))",
  left: "calc(8px + env(safe-area-inset-left, 0px))",
} as const;
