import { lazy, Suspense } from "react";
import { createBrowserRouter, Navigate, type RouteObject } from "react-router";
import { QueryError } from "@/shared/components/page";
import { AuthGate } from "./AuthGate";
import { SetupGate } from "./SetupGate";

import { NotFound } from "./shell/NotFound";
import { RouteError } from "./shell/RouteError";
import { AppShell } from "./shell/AppShell";
import { navigationForRoutes } from "./shell/nav-items";

const DashboardPage = lazy(() => import("@/features/dashboard/DashboardPage"));
const StacksPage = lazy(() => import("@/features/stacks/overview/StacksPage"));
const StackPage = lazy(() => import("@/features/stacks/detail/StackPage"));
const ContainersPage = lazy(() => import("@/features/containers/ContainersPage"));
const ContainerPage = lazy(() => import("@/features/containers/ContainerPage"));
const SecurityPage = lazy(() => import("@/features/security/SecurityPage"));
const NetworksPage = lazy(() => import("@/features/networks/NetworksPage"));
const HostsPage = lazy(() => import("@/features/hosts/HostsPage"));
const OnboardingPage = import.meta.env.DEV ? lazy(() => import("@/features/hosts/onboarding/OnboardingPage")) : null;
const VolumesPage = lazy(() => import("@/features/volumes/VolumesPage"));
const ImagesPage = lazy(() => import("@/features/images/ImagesPage"));
const VolumePage = lazy(() => import("@/features/volumes/VolumePage"));
const BackupsPage = lazy(() => import("@/features/volumes/BackupsPage"));
const HubBackupsPage = lazy(() => import("@/features/hub-backups/HubBackupsPage"));
const SettingsPage = lazy(() => import("@/features/settings/SettingsPage"));
const ProfilePage = lazy(() => import("@/features/settings/ProfilePage"));
const SetupPage = import.meta.env.DEV ? lazy(() => import("@/features/setup/SetupPage")) : null;
const LoginPage = lazy(() => import("@/features/auth/login/LoginPage"));
const PreviewPage = import.meta.env.DEV ? lazy(() => import("./preview/PreviewPage")) : null;

const shellRoutes: RouteObject[] = [
  {
    errorElement: <RouteError />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: "dashboard", element: <DashboardPage /> },
      { path: "stacks", element: <StacksPage /> },
      { path: "stacks/new", element: <StackPage /> },
      { path: "stacks/:stackId", element: <StackPage /> },
      { path: "containers", element: <ContainersPage /> },
      { path: "containers/:containerId", element: <ContainerPage /> },
      { path: "security", element: <SecurityPage /> },
      { path: "networks", element: <NetworksPage /> },
      { path: "hosts", element: <HostsPage /> },
      { path: "hosts/new", element: OnboardingPage ? <OnboardingPage /> : <QueryError /> },
      { path: "volumes", element: <VolumesPage /> },
      { path: "images", element: <ImagesPage /> },
      { path: "volumes/:volumeId", element: <VolumePage /> },
      { path: "backups", element: <BackupsPage /> },
      { path: "hub-backups", element: <HubBackupsPage /> },
      { path: "settings", element: <Navigate to="/settings/general" replace /> },
      { path: "settings/:section", element: <SettingsPage /> },
      { path: "profile", element: <ProfilePage /> },
      ...(PreviewPage ? [{ path: "preview/:section", element: <PreviewPage /> }] : []),
      { path: "*", element: <NotFound /> },
    ],
  },
];

export const navigation = navigationForRoutes(shellRoutes);

export const routes: RouteObject[] = [
  {
    // First-run setup is a full-screen preview outside the shell.
    path: "setup",
    errorElement: <RouteError />,
    element: (
      <Suspense fallback={null}>
        {SetupPage ? <SetupPage /> : <SetupGate />}
      </Suspense>
    ),
  },
  {
    // The build selects the login entry; authentication arrives with the backend.
    path: "login",
    errorElement: <RouteError />,
    element: (
      <Suspense fallback={null}>
        <LoginPage />
      </Suspense>
    ),
  },
  import.meta.env.DEV
    ? { element: <AppShell navigation={navigation} />, children: shellRoutes }
    : { element: <AuthGate />, children: [{ element: <AppShell navigation={navigation} />, children: shellRoutes }] },
];

export const router = createBrowserRouter(routes);
