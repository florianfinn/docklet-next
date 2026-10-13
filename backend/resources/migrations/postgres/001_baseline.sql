-- Source: docklet-next/backend/internal/schema/models.go
-- Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
-- Retrieved: 2026-10-13
-- License: Apache-2.0
-- Deviations: Generated PostgreSQL baseline from the retained GORM model registry.
-- Generated from backend/internal/schema.Models with PostgreSQL 18 pg_dump.
-- +goose Up
CREATE TABLE public.activities (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    environment_id text NOT NULL,
    batch_id text,
    type text NOT NULL,
    status text NOT NULL,
    resource_type text,
    resource_id text,
    resource_name text,
    progress bigint,
    step text,
    latest_message text,
    started_by_user_id text,
    started_by_username text,
    started_by_display_name text,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone,
    duration_ms bigint,
    error text,
    metadata text
);
CREATE TABLE public.activity_messages (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    activity_id text NOT NULL,
    level text NOT NULL,
    message text NOT NULL,
    payload text
);
CREATE TABLE public.api_key_permissions (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    api_key_id text NOT NULL,
    permission text NOT NULL,
    environment_id text
);
CREATE TABLE public.api_keys (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text NOT NULL,
    description text,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    managed_by text,
    kind text DEFAULT 'scoped'::text NOT NULL,
    user_id text,
    environment_id text,
    expires_at timestamp with time zone,
    last_used_at timestamp with time zone
);
CREATE TABLE public.auto_update_records (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    resource_id text,
    resource_type text,
    resource_name text,
    status text,
    start_time timestamp with time zone,
    end_time timestamp with time zone,
    update_available boolean,
    update_applied boolean,
    old_image_versions text,
    new_image_versions text,
    error text,
    details text
);
CREATE TABLE public.container_registries (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    url text,
    username text,
    token text,
    description text,
    insecure boolean,
    enabled boolean,
    registry_type text,
    repository_names text DEFAULT '[]'::text NOT NULL,
    aws_access_key_id text,
    aws_secret_access_key text,
    aws_region text,
    ecr_token text,
    ecr_token_generated_at timestamp with time zone
);
CREATE TABLE public.environment_update_jobs (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    status text,
    user_id text,
    username text,
    manager_version_at_start text,
    manager_digest_at_start text,
    manager_target_version text,
    results text,
    error text,
    completed_at timestamp with time zone
);
CREATE TABLE public.environments (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text,
    api_url text,
    status text,
    enabled boolean,
    is_edge boolean DEFAULT false,
    last_seen timestamp with time zone,
    last_edge_transport text,
    access_token text,
    api_key_id text
);
CREATE TABLE public.events (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deduplication_key text,
    type text,
    severity text,
    title text,
    description text,
    resource_type text,
    resource_id text,
    resource_name text,
    user_id text,
    username text,
    environment_id text,
    metadata text,
    "timestamp" timestamp with time zone
);
CREATE TABLE public.image_patches (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    environment_id text,
    original_image_id text,
    original_ref text,
    original_digest text,
    patched_ref text,
    mode text,
    status text,
    packages_updated bigint,
    error text,
    activity_id text,
    duration_ms bigint
);
CREATE TABLE public.image_updates (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    project_id text DEFAULT ''::text NOT NULL,
    service_name text DEFAULT ''::text NOT NULL,
    policy_key text DEFAULT ''::text NOT NULL,
    container_id text DEFAULT ''::text NOT NULL,
    image_id text DEFAULT ''::text NOT NULL,
    check_time timestamp with time zone,
    latest_version text,
    current_digest text,
    latest_digest text,
    last_error text,
    auth_method text,
    auth_username text,
    auth_registry text,
    repository text,
    tag text,
    update_type text,
    current_version text,
    response_time_ms bigint,
    has_update boolean,
    used_credential boolean,
    notification_sent boolean DEFAULT false
);
CREATE TABLE public.kv (
    key text NOT NULL,
    value text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.notification_settings (
    id bigint NOT NULL,
    provider character varying(50) NOT NULL,
    enabled boolean DEFAULT false,
    config jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE SEQUENCE public.notification_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.notification_settings_id_seq OWNED BY public.notification_settings.id;
CREATE TABLE public.project_tags (
    project_id text NOT NULL,
    name text NOT NULL,
    source text NOT NULL,
    color text DEFAULT 'gray'::text NOT NULL
);
CREATE TABLE public.projects (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text,
    dir_name text,
    path text,
    status text,
    status_reason text,
    service_count bigint,
    running_count bigint,
    compose_project_name text,
    image_refs_json text,
    build_image_refs_json text,
    is_archived boolean DEFAULT false,
    archived_at timestamp with time zone
);
CREATE TABLE public.roles (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text NOT NULL,
    description text,
    permissions text NOT NULL,
    built_in boolean DEFAULT false NOT NULL
);
CREATE TABLE public.settings (
    key text NOT NULL,
    value text
);
CREATE TABLE public.system_backup_policies (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    enabled boolean DEFAULT false NOT NULL,
    schedule text NOT NULL,
    retention_count bigint DEFAULT 7 NOT NULL,
    local_enabled boolean DEFAULT true NOT NULL
);
CREATE TABLE public.system_backup_recovery_config (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    encrypted_recovery_key text NOT NULL
);
CREATE TABLE public.system_backup_runs (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    size bigint,
    status text NOT NULL,
    trigger text NOT NULL,
    destination text NOT NULL,
    local_snapshot_id text,
    policy_id text,
    error text
);
CREATE TABLE public.user_avatars (
    user_id text NOT NULL,
    data bytea NOT NULL,
    mime_type text NOT NULL
);
CREATE TABLE public.user_role_assignments (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id text NOT NULL,
    role_id text NOT NULL,
    environment_id text,
    source text DEFAULT 'manual'::text NOT NULL
);
CREATE TABLE public.user_sessions (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    user_id text NOT NULL,
    refresh_token_hash text NOT NULL,
    user_agent text,
    ip_address text,
    source text,
    last_used_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone
);
CREATE TABLE public.users (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    username text,
    password_hash text,
    display_name text,
    email text,
    last_login timestamp with time zone,
    locale text,
    time_format text DEFAULT 'auto'::text NOT NULL,
    font_size bigint,
    preferences text,
    requires_password_change boolean,
    has_avatar boolean DEFAULT false NOT NULL
);
CREATE TABLE public.volume_backup_policies (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    volume_name text NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    schedule text NOT NULL,
    retention_count bigint DEFAULT 7 NOT NULL,
    stop_containers boolean DEFAULT false NOT NULL,
    local_enabled boolean NOT NULL
);
CREATE TABLE public.volume_backups (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    volume_name text,
    size bigint,
    status text DEFAULT 'succeeded'::text NOT NULL,
    trigger text DEFAULT 'manual'::text NOT NULL,
    destination text DEFAULT 'local'::text NOT NULL,
    format text DEFAULT 'archive'::text NOT NULL,
    local_snapshot_id text,
    policy_id text,
    error text
);
CREATE TABLE public.vulnerability_ignores (
    id text NOT NULL,
    environment_id text,
    image_id text,
    vulnerability_id text,
    pkg_name text,
    installed_version text,
    reason text,
    created_by text,
    created_at timestamp with time zone
);
CREATE TABLE public.vulnerability_reports (
    image_id text NOT NULL,
    data text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.vulnerability_risk_snapshots (
    snapshot_date text NOT NULL,
    scoring_version bigint,
    risk_score bigint,
    risk_band text,
    critical_count bigint,
    high_count bigint,
    medium_count bigint,
    low_count bigint,
    unknown_count bigint,
    findings_count bigint,
    known_exploited_count bigint,
    overdue_known_exploited_count bigint,
    high_epss_count bigint,
    exposed_critical_high_count bigint,
    fixable_count bigint,
    images_scanned bigint,
    images_total bigint,
    scored_images bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.vulnerability_scan_items (
    id bigint NOT NULL,
    image_id text,
    vulnerability_id text,
    pkg_name text,
    installed_version text,
    fixed_version text,
    severity text,
    severity_rank bigint,
    pkg_class text,
    pkg_type text,
    title text,
    cvss_score numeric,
    details text
);
CREATE SEQUENCE public.vulnerability_scan_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.vulnerability_scan_items_id_seq OWNED BY public.vulnerability_scan_items.id;
CREATE TABLE public.vulnerability_scans (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    image_name text,
    status text,
    scan_time timestamp with time zone,
    duration bigint,
    critical_count bigint,
    high_count bigint,
    medium_count bigint,
    low_count bigint,
    unknown_count bigint,
    total_count bigint,
    vulnerabilities text,
    error text,
    scanner_version text,
    fixable_count bigint,
    scan_policy text
);
CREATE TABLE public.vulnerability_threat_intel (
    cve_id text NOT NULL,
    known_exploited boolean,
    kev_date_added text,
    kev_due_date text,
    kev_ransomware boolean,
    epss_score numeric,
    epss_percentile numeric,
    epss_checked_at timestamp with time zone,
    updated_at timestamp with time zone
);
CREATE TABLE public.webhooks (
    id text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text NOT NULL,
    token_hash text NOT NULL,
    token_prefix text NOT NULL,
    target_type text NOT NULL,
    action_type text DEFAULT ''::text NOT NULL,
    target_id text NOT NULL,
    target_ref text DEFAULT ''::text NOT NULL,
    environment_id text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    last_triggered_at timestamp with time zone
);
ALTER TABLE ONLY public.notification_settings ALTER COLUMN id SET DEFAULT nextval('public.notification_settings_id_seq'::regclass);
ALTER TABLE ONLY public.vulnerability_scan_items ALTER COLUMN id SET DEFAULT nextval('public.vulnerability_scan_items_id_seq'::regclass);
ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.activity_messages
    ADD CONSTRAINT activity_messages_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.api_key_permissions
    ADD CONSTRAINT api_key_permissions_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.auto_update_records
    ADD CONSTRAINT auto_update_records_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.container_registries
    ADD CONSTRAINT container_registries_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.environment_update_jobs
    ADD CONSTRAINT environment_update_jobs_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.environments
    ADD CONSTRAINT environments_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.image_patches
    ADD CONSTRAINT image_patches_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.image_updates
    ADD CONSTRAINT image_updates_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.kv
    ADD CONSTRAINT kv_pkey PRIMARY KEY (key);
ALTER TABLE ONLY public.notification_settings
    ADD CONSTRAINT notification_settings_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.project_tags
    ADD CONSTRAINT project_tags_pkey PRIMARY KEY (project_id, name, source);
ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.settings
    ADD CONSTRAINT settings_pkey PRIMARY KEY (key);
ALTER TABLE ONLY public.system_backup_policies
    ADD CONSTRAINT system_backup_policies_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.system_backup_recovery_config
    ADD CONSTRAINT system_backup_recovery_config_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.system_backup_runs
    ADD CONSTRAINT system_backup_runs_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.user_avatars
    ADD CONSTRAINT user_avatars_pkey PRIMARY KEY (user_id);
ALTER TABLE ONLY public.user_role_assignments
    ADD CONSTRAINT user_role_assignments_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.volume_backup_policies
    ADD CONSTRAINT volume_backup_policies_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.volume_backups
    ADD CONSTRAINT volume_backups_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_ignores
    ADD CONSTRAINT vulnerability_ignores_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_reports
    ADD CONSTRAINT vulnerability_reports_pkey PRIMARY KEY (image_id);
ALTER TABLE ONLY public.vulnerability_risk_snapshots
    ADD CONSTRAINT vulnerability_risk_snapshots_pkey PRIMARY KEY (snapshot_date);
ALTER TABLE ONLY public.vulnerability_scan_items
    ADD CONSTRAINT vulnerability_scan_items_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_scans
    ADD CONSTRAINT vulnerability_scans_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.vulnerability_threat_intel
    ADD CONSTRAINT vulnerability_threat_intel_pkey PRIMARY KEY (cve_id);
ALTER TABLE ONLY public.webhooks
    ADD CONSTRAINT webhooks_pkey PRIMARY KEY (id);
CREATE INDEX idx_activities_batch_id ON public.activities USING btree (batch_id);
CREATE INDEX idx_activities_environment_id ON public.activities USING btree (environment_id);
CREATE INDEX idx_activities_resource_id ON public.activities USING btree (resource_id);
CREATE INDEX idx_activities_resource_type ON public.activities USING btree (resource_type);
CREATE INDEX idx_activities_started_by_user_id ON public.activities USING btree (started_by_user_id);
CREATE INDEX idx_activities_status ON public.activities USING btree (status);
CREATE INDEX idx_activities_type ON public.activities USING btree (type);
CREATE INDEX idx_activity_messages_activity_id ON public.activity_messages USING btree (activity_id);
CREATE INDEX idx_api_key_permissions_api_key_id ON public.api_key_permissions USING btree (api_key_id);
CREATE UNIQUE INDEX idx_events_deduplication_key ON public.events USING btree (deduplication_key);
CREATE INDEX idx_image_patches_env_image ON public.image_patches USING btree (environment_id, original_image_id);
CREATE INDEX idx_image_updates_container_id ON public.image_updates USING btree (container_id);
CREATE INDEX idx_image_updates_image_id ON public.image_updates USING btree (image_id);
CREATE INDEX idx_image_updates_project_id ON public.image_updates USING btree (project_id);
CREATE INDEX idx_notification_settings_provider ON public.notification_settings USING btree (provider);
CREATE INDEX idx_projects_is_archived ON public.projects USING btree (is_archived);
CREATE INDEX idx_projects_name ON public.projects USING btree (name);
CREATE UNIQUE INDEX idx_projects_path ON public.projects USING btree (path);
CREATE UNIQUE INDEX idx_roles_name ON public.roles USING btree (name);
CREATE INDEX idx_system_backup_runs_policy_id ON public.system_backup_runs USING btree (policy_id);
CREATE INDEX idx_user_role_assignments_environment_id ON public.user_role_assignments USING btree (environment_id);
CREATE INDEX idx_user_role_assignments_role_id ON public.user_role_assignments USING btree (role_id);
CREATE INDEX idx_user_role_assignments_user_id ON public.user_role_assignments USING btree (user_id);
CREATE INDEX idx_user_sessions_expires_at ON public.user_sessions USING btree (expires_at);
CREATE UNIQUE INDEX idx_user_sessions_refresh_token_hash ON public.user_sessions USING btree (refresh_token_hash);
CREATE INDEX idx_user_sessions_user_id ON public.user_sessions USING btree (user_id);
CREATE INDEX idx_volume_backup_policies_volume_name ON public.volume_backup_policies USING btree (volume_name);
CREATE INDEX idx_volume_backups_policy_id ON public.volume_backups USING btree (policy_id);
CREATE INDEX idx_volume_backups_volume_name ON public.volume_backups USING btree (volume_name);
CREATE INDEX idx_vulnerability_ignores_environment_id ON public.vulnerability_ignores USING btree (environment_id);
CREATE INDEX idx_vulnerability_ignores_image_id ON public.vulnerability_ignores USING btree (image_id);
CREATE INDEX idx_vulnerability_ignores_vulnerability_id ON public.vulnerability_ignores USING btree (vulnerability_id);
CREATE INDEX idx_vulnerability_scan_items_image_id ON public.vulnerability_scan_items USING btree (image_id);
CREATE INDEX idx_vulnerability_scan_items_severity ON public.vulnerability_scan_items USING btree (severity);
CREATE INDEX idx_vulnerability_scan_items_vulnerability_id ON public.vulnerability_scan_items USING btree (vulnerability_id);
CREATE UNIQUE INDEX idx_webhooks_token_hash ON public.webhooks USING btree (token_hash);
/*
* Source: getarcaneapp/arcane/backend/resources/migrations/postgres
* Revision: 095cb778daba1034e2a39961199e712a75d4ba29
* Retrieved: 2026-10-13
* License: BSD-3-Clause
* Deviations: Retained-table constraints and indexes filtered for the functional cut.
*/
ALTER TABLE public."activities" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."activity_messages" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."api_key_permissions" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."api_key_permissions" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."api_keys" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."api_keys" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."auto_update_records" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."auto_update_records" ALTER COLUMN "resource_id" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "resource_name" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "resource_type" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "start_time" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "status" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "update_applied" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "update_applied" SET DEFAULT false;
ALTER TABLE public."auto_update_records" ALTER COLUMN "update_available" SET NOT NULL;
ALTER TABLE public."auto_update_records" ALTER COLUMN "update_available" SET DEFAULT false;
ALTER TABLE public."container_registries" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."container_registries" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."container_registries" ALTER COLUMN "enabled" SET NOT NULL;
ALTER TABLE public."container_registries" ALTER COLUMN "enabled" SET DEFAULT true;
ALTER TABLE public."container_registries" ALTER COLUMN "insecure" SET NOT NULL;
ALTER TABLE public."container_registries" ALTER COLUMN "insecure" SET DEFAULT false;
ALTER TABLE public."container_registries" ALTER COLUMN "registry_type" SET NOT NULL;
ALTER TABLE public."container_registries" ALTER COLUMN "registry_type" SET DEFAULT 'generic'::text;
ALTER TABLE public."container_registries" ALTER COLUMN "token" SET NOT NULL;
ALTER TABLE public."container_registries" ALTER COLUMN "url" SET NOT NULL;
ALTER TABLE public."container_registries" ALTER COLUMN "username" SET NOT NULL;
ALTER TABLE public."environment_update_jobs" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."environment_update_jobs" ALTER COLUMN "status" SET NOT NULL;
ALTER TABLE public."environments" ALTER COLUMN "api_url" SET NOT NULL;
ALTER TABLE public."environments" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."environments" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."environments" ALTER COLUMN "enabled" SET NOT NULL;
ALTER TABLE public."environments" ALTER COLUMN "enabled" SET DEFAULT true;
ALTER TABLE public."environments" ALTER COLUMN "is_edge" SET NOT NULL;
ALTER TABLE public."environments" ALTER COLUMN "status" SET NOT NULL;
ALTER TABLE public."environments" ALTER COLUMN "status" SET DEFAULT 'offline'::text;
ALTER TABLE public."events" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."events" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."events" ALTER COLUMN "severity" SET NOT NULL;
ALTER TABLE public."events" ALTER COLUMN "severity" SET DEFAULT 'info'::text;
ALTER TABLE public."events" ALTER COLUMN "timestamp" SET NOT NULL;
ALTER TABLE public."events" ALTER COLUMN "title" SET NOT NULL;
ALTER TABLE public."events" ALTER COLUMN "type" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."image_patches" ALTER COLUMN "environment_id" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "mode" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "original_image_id" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "original_ref" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "patched_ref" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "status" SET NOT NULL;
ALTER TABLE public."image_patches" ALTER COLUMN "status" SET DEFAULT 'patching'::text;
ALTER TABLE public."image_updates" ALTER COLUMN "check_time" SET NOT NULL;
ALTER TABLE public."image_updates" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."image_updates" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."image_updates" ALTER COLUMN "current_version" SET NOT NULL;
ALTER TABLE public."image_updates" ALTER COLUMN "has_update" SET NOT NULL;
ALTER TABLE public."image_updates" ALTER COLUMN "has_update" SET DEFAULT false;
ALTER TABLE public."image_updates" ALTER COLUMN "repository" SET NOT NULL;
ALTER TABLE public."image_updates" ALTER COLUMN "response_time_ms" SET NOT NULL;
ALTER TABLE public."image_updates" ALTER COLUMN "response_time_ms" SET DEFAULT 0;
ALTER TABLE public."image_updates" ALTER COLUMN "tag" SET NOT NULL;
ALTER TABLE public."image_updates" ALTER COLUMN "used_credential" SET DEFAULT false;
ALTER TABLE public."kv" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."kv" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."kv" ALTER COLUMN "value" SET NOT NULL;
ALTER TABLE public."notification_settings" ALTER COLUMN "config" SET NOT NULL;
ALTER TABLE public."notification_settings" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."notification_settings" ALTER COLUMN "updated_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."projects" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."projects" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."projects" ALTER COLUMN "image_refs_json" SET NOT NULL;
ALTER TABLE public."projects" ALTER COLUMN "image_refs_json" SET DEFAULT ''::text;
ALTER TABLE public."projects" ALTER COLUMN "is_archived" SET NOT NULL;
ALTER TABLE public."projects" ALTER COLUMN "name" SET NOT NULL;
ALTER TABLE public."projects" ALTER COLUMN "path" SET NOT NULL;
ALTER TABLE public."projects" ALTER COLUMN "running_count" SET NOT NULL;
ALTER TABLE public."projects" ALTER COLUMN "running_count" SET DEFAULT 0;
ALTER TABLE public."projects" ALTER COLUMN "service_count" SET NOT NULL;
ALTER TABLE public."projects" ALTER COLUMN "service_count" SET DEFAULT 0;
ALTER TABLE public."projects" ALTER COLUMN "status" SET NOT NULL;
ALTER TABLE public."roles" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."roles" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."roles" ALTER COLUMN "permissions" SET DEFAULT '[]'::jsonb;
ALTER TABLE public."settings" ALTER COLUMN "value" SET NOT NULL;
ALTER TABLE public."system_backup_policies" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."system_backup_policies" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."system_backup_recovery_config" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."system_backup_recovery_config" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."system_backup_runs" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."system_backup_runs" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."system_backup_runs" ALTER COLUMN "size" SET NOT NULL;
ALTER TABLE public."system_backup_runs" ALTER COLUMN "size" SET DEFAULT 0;
ALTER TABLE public."user_role_assignments" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."user_role_assignments" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."user_sessions" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."users" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."users" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."users" ALTER COLUMN "password_hash" SET NOT NULL;
ALTER TABLE public."users" ALTER COLUMN "requires_password_change" SET NOT NULL;
ALTER TABLE public."users" ALTER COLUMN "requires_password_change" SET DEFAULT false;
ALTER TABLE public."users" ALTER COLUMN "username" SET NOT NULL;
ALTER TABLE public."volume_backup_policies" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."volume_backup_policies" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."volume_backup_policies" ALTER COLUMN "local_enabled" SET DEFAULT true;
ALTER TABLE public."volume_backups" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."volume_backups" ALTER COLUMN "size" SET NOT NULL;
ALTER TABLE public."volume_backups" ALTER COLUMN "volume_name" SET NOT NULL;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "created_by" SET NOT NULL;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "environment_id" SET NOT NULL;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "image_id" SET NOT NULL;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "installed_version" SET NOT NULL;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "pkg_name" SET NOT NULL;
ALTER TABLE public."vulnerability_ignores" ALTER COLUMN "vulnerability_id" SET NOT NULL;
ALTER TABLE public."vulnerability_reports" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."vulnerability_reports" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."vulnerability_reports" ALTER COLUMN "data" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "critical_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "critical_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "exposed_critical_high_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "exposed_critical_high_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "findings_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "findings_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "fixable_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "fixable_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "high_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "high_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "high_epss_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "high_epss_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "images_scanned" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "images_scanned" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "images_total" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "images_total" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "known_exploited_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "known_exploited_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "low_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "low_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "medium_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "medium_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "overdue_known_exploited_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "overdue_known_exploited_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "risk_band" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "risk_band" SET DEFAULT 'none'::text;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "risk_score" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "risk_score" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "scored_images" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "scored_images" SET DEFAULT 0;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "scoring_version" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "scoring_version" SET DEFAULT 1;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "unknown_count" SET NOT NULL;
ALTER TABLE public."vulnerability_risk_snapshots" ALTER COLUMN "unknown_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "fixed_version" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "fixed_version" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "image_id" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "installed_version" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "installed_version" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "pkg_class" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "pkg_class" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "pkg_name" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "pkg_type" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "pkg_type" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "severity" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "severity_rank" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "severity_rank" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "title" SET NOT NULL;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "title" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_scan_items" ALTER COLUMN "vulnerability_id" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "created_at" SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "critical_count" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "critical_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "duration" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "duration" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "high_count" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "high_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "image_name" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "low_count" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "low_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "medium_count" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "medium_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "scan_policy" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "scan_policy" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "scan_time" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "status" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "status" SET DEFAULT 'pending'::text;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "total_count" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "total_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "unknown_count" SET NOT NULL;
ALTER TABLE public."vulnerability_scans" ALTER COLUMN "unknown_count" SET DEFAULT 0;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "kev_date_added" SET NOT NULL;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "kev_date_added" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "kev_due_date" SET NOT NULL;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "kev_due_date" SET DEFAULT ''::text;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "kev_ransomware" SET NOT NULL;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "kev_ransomware" SET DEFAULT false;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "known_exploited" SET NOT NULL;
ALTER TABLE public."vulnerability_threat_intel" ALTER COLUMN "known_exploited" SET DEFAULT false;
ALTER TABLE public."webhooks" ALTER COLUMN "created_at" SET NOT NULL;
ALTER TABLE public."webhooks" ALTER COLUMN "created_at" SET DEFAULT now();
ALTER TABLE public."activity_messages" ADD CONSTRAINT "activity_messages_activity_id_fkey" FOREIGN KEY (activity_id) REFERENCES activities(id) ON DELETE CASCADE;
ALTER TABLE public."api_key_permissions" ADD CONSTRAINT "api_key_permissions_api_key_id_fkey" FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE;
ALTER TABLE public."api_key_permissions" ADD CONSTRAINT "api_key_permissions_environment_id_fkey" FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE CASCADE;
ALTER TABLE public."api_keys" ADD CONSTRAINT "api_keys_environment_id_fkey" FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE CASCADE;
ALTER TABLE public."api_keys" ADD CONSTRAINT "api_keys_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE public."environments" ADD CONSTRAINT "environments_api_key_id_fkey" FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL;
ALTER TABLE public."project_tags" ADD CONSTRAINT "project_tags_color_check" CHECK ((color = ANY (ARRAY['gray'::text, 'purple'::text, 'blue'::text, 'green'::text, 'yellow'::text, 'orange'::text, 'red'::text, 'pink'::text])));
ALTER TABLE public."project_tags" ADD CONSTRAINT "project_tags_project_id_fkey" FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
ALTER TABLE public."project_tags" ADD CONSTRAINT "project_tags_source_check" CHECK ((source = ANY (ARRAY['ui'::text, 'compose'::text])));
ALTER TABLE public."projects" ADD CONSTRAINT "stacks_table_dir_name_key" UNIQUE (dir_name);
ALTER TABLE public."roles" ADD CONSTRAINT "roles_name_key" UNIQUE (name);
ALTER TABLE public."user_avatars" ADD CONSTRAINT "user_avatars_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE public."user_role_assignments" ADD CONSTRAINT "user_role_assignments_environment_id_fkey" FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE CASCADE;
ALTER TABLE public."user_role_assignments" ADD CONSTRAINT "user_role_assignments_role_id_fkey" FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
ALTER TABLE public."user_role_assignments" ADD CONSTRAINT "user_role_assignments_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE public."user_sessions" ADD CONSTRAINT "user_sessions_refresh_token_hash_key" UNIQUE (refresh_token_hash);
ALTER TABLE public."user_sessions" ADD CONSTRAINT "user_sessions_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE public."users" ADD CONSTRAINT "users_table_username_key" UNIQUE (username);
ALTER TABLE public."users" ADD CONSTRAINT "users_time_format_check" CHECK ((time_format = ANY (ARRAY['auto'::text, '12h'::text, '24h'::text])));
ALTER TABLE public."vulnerability_scan_items" ADD CONSTRAINT "vulnerability_scan_items_image_id_fkey" FOREIGN KEY (image_id) REFERENCES vulnerability_scans(id) ON DELETE CASCADE;
ALTER TABLE public."webhooks" ADD CONSTRAINT "webhooks_token_hash_key" UNIQUE (token_hash);
CREATE INDEX idx_activities_environment_batch ON public.activities USING btree (environment_id, batch_id);
CREATE INDEX idx_activities_environment_started ON public.activities USING btree (environment_id, started_at);
CREATE INDEX idx_activities_environment_status_updated ON public.activities USING btree (environment_id, status, updated_at);
CREATE INDEX idx_activities_resource ON public.activities USING btree (resource_type, resource_id);
CREATE INDEX idx_activity_messages_activity_created ON public.activity_messages USING btree (activity_id, created_at);
CREATE UNIQUE INDEX idx_akp_uniq ON public.api_key_permissions USING btree (api_key_id, permission, COALESCE(environment_id, ''::text));
CREATE INDEX idx_api_keys_expires_at_not_null ON public.api_keys USING btree (expires_at) WHERE (expires_at IS NOT NULL);
CREATE INDEX idx_api_keys_key_hash ON public.api_keys USING btree (key_hash);
CREATE INDEX idx_api_keys_key_prefix ON public.api_keys USING btree (key_prefix);
CREATE INDEX idx_api_keys_user_managed_by_created_at ON public.api_keys USING btree (user_id, managed_by, created_at, id);
CREATE INDEX idx_auto_update_resource_id ON public.auto_update_records USING btree (resource_id);
CREATE INDEX idx_auto_update_start_time ON public.auto_update_records USING btree (start_time);
CREATE INDEX idx_container_registries_enabled ON public.container_registries USING btree (enabled);
CREATE INDEX idx_environment_update_jobs_status ON public.environment_update_jobs USING btree (status);
CREATE INDEX idx_environments_access_token_not_null ON public.environments USING btree (access_token) WHERE (access_token IS NOT NULL);
CREATE INDEX idx_environments_api_url ON public.environments USING btree (api_url);
CREATE INDEX idx_environments_enabled ON public.environments USING btree (enabled);
CREATE INDEX idx_environments_enabled_true ON public.environments USING btree (id, name) WHERE (enabled = true);
CREATE INDEX idx_environments_last_seen ON public.environments USING btree (last_seen);
CREATE INDEX idx_environments_name ON public.environments USING btree (name);
CREATE INDEX idx_environments_status ON public.environments USING btree (status);
CREATE INDEX idx_events_environment_timestamp ON public.events USING btree (environment_id, "timestamp");
CREATE INDEX idx_events_resource_id ON public.events USING btree (resource_id);
CREATE INDEX idx_events_resource_type ON public.events USING btree (resource_type);
CREATE INDEX idx_events_severity ON public.events USING btree (severity);
CREATE INDEX idx_events_timestamp ON public.events USING btree ("timestamp");
CREATE INDEX idx_events_type ON public.events USING btree (type);
CREATE INDEX idx_events_user_id ON public.events USING btree (user_id);
CREATE INDEX idx_image_patches_created_at ON public.image_patches USING btree (created_at);
CREATE INDEX idx_image_updates_check_time ON public.image_updates USING btree (check_time);
CREATE INDEX idx_image_updates_has_update ON public.image_updates USING btree (has_update);
CREATE INDEX idx_image_updates_has_update_notification ON public.image_updates USING btree (has_update, notification_sent);
CREATE INDEX idx_image_updates_notification_sent ON public.image_updates USING btree (notification_sent);
CREATE INDEX idx_image_updates_repository_tag ON public.image_updates USING btree (repository, tag);
CREATE INDEX idx_notification_settings_enabled ON public.notification_settings USING btree (enabled);
CREATE INDEX idx_project_tags_name ON public.project_tags USING btree (name);
CREATE INDEX idx_projects_dir_name_not_null ON public.projects USING btree (dir_name) WHERE (dir_name IS NOT NULL);
CREATE INDEX idx_projects_status ON public.projects USING btree (status);
CREATE UNIQUE INDEX idx_ura_uniq ON public.user_role_assignments USING btree (user_id, role_id, COALESCE(environment_id, ''::text));
CREATE INDEX idx_users_email ON public.users USING btree (email);
CREATE INDEX idx_users_username ON public.users USING btree (username);
CREATE INDEX idx_volume_backups_volume_name_created_at ON public.volume_backups USING btree (volume_name, created_at);
CREATE INDEX idx_vulnerability_ignores_composite ON public.vulnerability_ignores USING btree (environment_id, image_id, vulnerability_id, pkg_name, installed_version);
CREATE INDEX idx_vulnerability_ignores_env_created_at ON public.vulnerability_ignores USING btree (environment_id, created_at);
CREATE INDEX idx_vulnerability_ignores_env_vulnerability_id ON public.vulnerability_ignores USING btree (environment_id, vulnerability_id);
CREATE INDEX idx_vulnerability_scan_items_severity_rank_id ON public.vulnerability_scan_items USING btree (severity_rank, id);
CREATE INDEX idx_vulnerability_scans_scan_time ON public.vulnerability_scans USING btree (scan_time);
CREATE INDEX idx_vulnerability_scans_status_total_count ON public.vulnerability_scans USING btree (status, total_count);
CREATE INDEX idx_vulnerability_threat_intel_known_exploited ON public.vulnerability_threat_intel USING btree (known_exploited);
CREATE INDEX idx_webhooks_environment_id ON public.webhooks USING btree (environment_id);
CREATE INDEX idx_webhooks_token_prefix ON public.webhooks USING btree (token_prefix);
