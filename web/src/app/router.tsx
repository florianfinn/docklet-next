import { lazy, Suspense } from "react";
import { createBrowserRouter, Navigate } from "react-router";

import { NotFound } from "./shell/NotFound";
import { RouteError } from "./shell/RouteError";
import { AppShell } from "./shell/AppShell";

const DashboardPage = lazy(() => import("@/features/dashboard/DashboardPage"));
const StacksPage = lazy(() => import("@/features/stacks/overview/StacksPage"));
const StackPage = lazy(() => import("@/features/stacks/detail/StackPage"));
const ContainersPage = lazy(() => import("@/features/containers/ContainersPage"));
const ContainerPage = lazy(() => import("@/features/containers/ContainerPage"));
const SecurityPage = lazy(() => import("@/features/security/SecurityPage"));
const NetworksPage = lazy(() => import("@/features/networks/NetworksPage"));
const HostsPage = lazy(() => import("@/features/hosts/HostsPage"));
const OnboardingPage = lazy(() => import("@/features/hosts/onboarding/OnboardingPage"));
const VolumesPage = lazy(() => import("@/features/volumes/VolumesPage"));
const ImagesPage = lazy(() => import("@/features/images/ImagesPage"));
const VolumePage = lazy(() => import("@/features/volumes/VolumePage"));
const BackupsPage = lazy(() => import("@/features/volumes/BackupsPage"));
const SettingsPage = lazy(() => import("@/features/settings/SettingsPage"));
const ProfilePage = lazy(() => import("@/features/settings/ProfilePage"));
const SetupPage = lazy(() => import("@/features/setup/SetupPage"));
const LoginPage = lazy(() => import("@/features/auth/login/LoginPage"));
const PreviewPage = lazy(() => import("./preview/PreviewPage"));

export const routes = [
  {
    // First-run setup is a full-screen preview outside the shell.
    path: "setup",
    errorElement: <RouteError />,
    element: (
      <Suspense fallback={null}>
        <SetupPage />
      </Suspense>
    ),
  },
  {
    // Login demo: full screen like setup; it guards no route.
    path: "login",
    errorElement: <RouteError />,
    element: (
      <Suspense fallback={null}>
        <LoginPage />
      </Suspense>
    ),
  },
  {
    element: <AppShell />,
    children: [
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
          { path: "hosts/new", element: <OnboardingPage /> },
          { path: "volumes", element: <VolumesPage /> },
          { path: "images", element: <ImagesPage /> },
          { path: "volumes/:volumeId", element: <VolumePage /> },
          { path: "backups", element: <BackupsPage /> },
          { path: "settings", element: <Navigate to="/settings/general" replace /> },
          { path: "settings/:section", element: <SettingsPage /> },
          { path: "profile", element: <ProfilePage /> },
          { path: "preview/:section", element: <PreviewPage /> },
          { path: "*", element: <NotFound /> },
        ],
      },
    ],
  },
];

export const router = createBrowserRouter(routes);
