// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/app/router.tsx
// Revision: 437b7e748319217bc87288adedb804637e03e74f20c0008b9ad08a5a411b5367
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: M2 production shell: disconnected error handling, development-only demo boundary, neutral product text or responsive layout. Historical import recorded in web/PROVENANCE.md; M2 review follow-up for production route, navigation and boundary coverage. See web/PROVENANCE.md. #69 connected first-run identity flow. M5 #84 part 2: live host registration, credential lifecycle and admin production onboarding. Production image/registry UI and validation for #230. Simple-user management, assigned-stack access and mode-aware self-service. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). M21 #437: advisory detail.

import { lazy, Suspense } from "react";
import { createBrowserRouter, Navigate, Outlet, type RouteObject } from "react-router";
import { useIsUser } from "@/data/use-principal";
import { useAdapter } from "@/data/use-adapter";
import { AuthGate } from "./AuthGate";
import { SetupGate } from "./SetupGate";
import ProductLoginPage from "@/features/auth/login/ProductLoginPage";

import { NotFound } from "./shell/NotFound";
import { RouteError } from "./shell/RouteError";
import { AppShell } from "./shell/AppShell";
import { navigationForRoutes } from "./shell/nav-items";

const DashboardPage = lazy(() => import("@/features/dashboard/DashboardPage"));
const UserDashboard = lazy(() => import("@/features/dashboard/UserDashboard"));
const StacksPage = lazy(() => import("@/features/stacks/overview/StacksPage"));
const StackPage = lazy(() => import("@/features/stacks/detail/StackPage"));
const ContainersPage = lazy(() => import("@/features/containers/ContainersPage"));
const ContainerPage = lazy(() => import("@/features/containers/ContainerPage"));
const VulnerabilityPage = lazy(() => import("@/features/security/VulnerabilityPage"));
const SecurityPage = lazy(() => import("@/features/security/SecurityPage"));
const NetworksPage = lazy(() => import("@/features/networks/NetworksPage"));
const HostsPage = lazy(() => import("@/features/hosts/HostsPage"));
const OnboardingPage = lazy(() => import("@/features/hosts/onboarding/OnboardingPage"));
const VolumesPage = lazy(() => import("@/features/volumes/VolumesPage"));
const RegistriesPage = lazy(() => import("@/features/registries/RegistriesPage"));
const ImagesPage = lazy(() => import("@/features/images/ImagesPage"));
const VolumePage = lazy(() => import("@/features/volumes/VolumePage"));
const BackupsPage = lazy(() => import("@/features/volumes/BackupsPage"));
const HubBackupsPage = lazy(() => import("@/features/hub-backups/HubBackupsPage"));
const SettingsPage = lazy(() => import("@/features/settings/SettingsPage"));
const ProfilePage = lazy(() => import("@/features/settings/ProfilePage"));
const SetupPage = lazy(() => import("@/features/setup/SetupPage"));
const DemoLoginPage = lazy(() => import("@/features/auth/login/LoginPage"));
const ProxyPage = import.meta.env.DEV ? lazy(() => import("@/features/design-lab/proxy/ProxyPage")) : null;
const ComingSoonPage = lazy(() => import("./coming-soon/ComingSoonPage"));

function HubRouteGate() {
  return useAdapter().kind === "http" ? <AuthGate /> : <Outlet />;
}

function SetupRoute() {
  return useAdapter().kind === "http" ? <SetupGate /> : <SetupPage />;
}

function LoginRoute() {
  return useAdapter().kind === "http" ? <ProductLoginPage /> : <DemoLoginPage />;
}

function LandingRoute() { return <Navigate to="/dashboard" replace />; }

function DashboardRoute() { return useIsUser() ? <UserDashboard /> : <DashboardPage />; }

const shellRoutes: RouteObject[] = [
  {
    errorElement: <RouteError />,
    children: [
      { index: true, element: <LandingRoute /> },
      { path: "dashboard", element: <DashboardRoute /> },
      { path: "stacks", element: <StacksPage /> },
      { path: "stacks/new", element: <StackPage /> },
      { path: "stacks/:stackId", element: <StackPage /> },
      { path: "containers", element: <ContainersPage /> },
      { path: "containers/:containerId", element: <ContainerPage /> },
      { path: "security", element: <SecurityPage /> },
      { path: "security/vulnerabilities/:key", element: <VulnerabilityPage /> },
      { path: "networks", element: <NetworksPage /> },
      { path: "hosts", element: <HostsPage /> },
      { path: "hosts/new", element: <OnboardingPage /> },
      { path: "volumes", element: <VolumesPage /> },
      { path: "images", element: <ImagesPage /> },
      { path: "registries", element: <RegistriesPage /> },
      { path: "volumes/:volumeId", element: <VolumePage /> },
      { path: "backups", element: <BackupsPage /> },
      { path: "hub-backups", element: <HubBackupsPage /> },
      { path: "settings", element: <Navigate to="/settings/general" replace /> },
      { path: "settings/:section", element: <SettingsPage /> },
      { path: "profile", element: <ProfilePage /> },
      { path: "proxy", element: ProxyPage ? <ProxyPage /> : <ComingSoonPage area="proxy" /> },
      { path: "games", element: <ComingSoonPage area="games" /> },
      { path: "*", element: <NotFound /> },
    ],
  },
];

export const navigation = navigationForRoutes(shellRoutes);

export const routes: RouteObject[] = [
  {
    // Setup remains outside the shell in both connected and demo routes.
    path: "setup",
    errorElement: <RouteError />,
    element: (
      <Suspense fallback={null}>
        <SetupRoute />
      </Suspense>
    ),
  },
  {
    // The adapter selects the connected or demo sign-in flow.
    path: "login",
    errorElement: <RouteError />,
    element: (
      <Suspense fallback={null}>
        <LoginRoute />
      </Suspense>
    ),
  },
  { element: <HubRouteGate />, children: [{ element: <AppShell navigation={navigation} />, children: shellRoutes }] },
];

export const router = createBrowserRouter(routes);
