// Source: getarcaneapp/arcane/backend/internal/settings/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Functional cut: removed APNS mobile push, template catalogue, federated credentials, GitOps, Swarm, S3 backup destinations, standalone build workshop, OIDC and passkeys. #69 first-run setup and hub settings persistence; Complete helper image setting.

package settings

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.getarcane.app/kit/pkg"
)

const (
	SettingVisibilityPublic   SettingVisibility = 0
	SettingVisibilityNonAdmin SettingVisibility = 1
	SettingVisibilityAll      SettingVisibility = 2

	redactionMask = "XXXXXXXXXX"
)

type SettingVariable struct {
	Key   string `gorm:"primaryKey"`
	Value string
}

type SettingVisibility int

type settingFieldMeta struct {
	index               int
	key                 string
	attrs               string
	isPublic            bool
	isVisibleToNonAdmin bool
	isSensitive         bool
}

var settingsFieldCache struct {
	once    sync.Once
	ordered []settingFieldMeta
	byKey   map[string]settingFieldMeta
}

// IsTrue returns true if the value is a truthy string
func (s SettingVariable) IsTrue() bool {
	ok, _ := strconv.ParseBool(s.Value)
	return ok
}

// AsInt returns the value as an integer
func (s SettingVariable) AsInt() int {
	val, _ := strconv.Atoi(s.Value)
	return val
}

// AsDurationSeconds returns the value as a time.Duration in seconds
func (s SettingVariable) AsDurationSeconds() time.Duration {
	val, err := strconv.Atoi(s.Value)
	return kit.Ternary(err != nil, 0, time.Duration(val)*time.Second)
}

//nolint:lll // Reflection metadata must remain in single struct-tag literals.
type Settings struct {
	// General category
	ProjectsDirectory           SettingVariable `key:"projectsDirectory,envOverride" meta:"label=Projects Directory;type=text;keywords=projects,directory,path,folder,location,storage,files,compose,docker-compose;category=internal;description=Configure where project files are stored"`
	FollowProjectSymlinks       SettingVariable `key:"followProjectSymlinks,envOverride" meta:"label=Follow Project Symlinks;type=boolean;keywords=projects,symlink,symlinks,symbolic links,compose,directory,discovery;category=general;description=Treat symlinked child directories inside the projects directory as Docker Compose projects"`
	DiskUsagePath               SettingVariable `key:"diskUsagePath" meta:"label=Disk Usage Path;type=text;keywords=disk,usage,path,storage,folder,files;category=general;description=Path used for disk usage calculations"`
	BaseServerURL               SettingVariable `key:"baseServerUrl" meta:"label=Base Server URL;type=text;keywords=base,url,server,domain,host,endpoint,address,link;category=general;description=Set the base URL for the application"`
	SetupCompleted              SettingVariable `key:"setupCompleted" meta:"label=Setup Completed;type=boolean;category=internal;description=Whether first-run setup has finished"`
	HubName                     SettingVariable `key:"hubName" meta:"label=Hub Name;type=text;category=general;description=Name displayed for this hub"`
	HubAddress                  SettingVariable `key:"hubAddress" meta:"label=Hub Address;type=text;category=general;description=Address agents use to reach this hub"`
	DomainURL                   SettingVariable `key:"domainUrl" meta:"label=Domain URL;type=text;category=general;description=Optional public hub URL"`
	LinkBase                    SettingVariable `key:"linkBase" meta:"label=Link Base;type=select;category=general;description=Which hub address links use"`
	AgentProjectsDirectory      SettingVariable `key:"agentProjectsDirectory" meta:"label=Agent Projects Directory;type=text;category=general;description=Default project directory on agent hosts"`
	AgentInstallDirectory       SettingVariable `key:"agentInstallDirectory" meta:"label=Agent Install Directory;type=text;category=general;description=Default agent installation directory"`
	DefaultHostConnection       SettingVariable `key:"defaultHostConnection" meta:"label=Default Host Connection;type=select;category=general;description=Default host connection method"`
	DefaultPUID                 SettingVariable `key:"defaultPuid" meta:"label=Default PUID;type=text;category=general;description=Default file owner user ID"`
	DefaultPGID                 SettingVariable `key:"defaultPgid" meta:"label=Default PGID;type=text;category=general;description=Default file owner group ID"`
	DetectSocketGroup           SettingVariable `key:"detectSocketGroup" meta:"label=Detect Socket Group;type=boolean;category=general;description=Detect agent Docker socket group"`
	EnableGravatar              SettingVariable `key:"enableGravatar,authrequired" meta:"label=Enable Gravatar;type=boolean;keywords=gravatar,avatar,profile,picture,image,user,photo;category=users;description=Enable Gravatar profile pictures for users"`
	AvatarMaxUploadSizeMb       SettingVariable `key:"avatarMaxUploadSizeMb,authrequired" meta:"label=Avatar Max Upload Size (MB);type=number;keywords=avatar,profile,picture,upload,size,limit,maximum,image,user,photo,mb;category=users;description=Maximum size in MB for profile picture uploads (default: 2)"`
	DefaultShell                SettingVariable `key:"defaultShell" meta:"label=Default Shell;type=text;keywords=shell,default,shellpath,path,login;category=general;description=Default shell to use for commands"`
	ExperimentalFeaturesEnabled SettingVariable `key:"experimentalFeaturesEnabled,authrequired" meta:"label=Experimental Features;type=boolean;keywords=experimental,features,beta,preview,flags;category=general;description=Enable experimental features that may change or be removed in future releases"`
	EnvironmentHealthInterval   SettingVariable `key:"environmentHealthInterval" meta:"label=Environment Health Check Interval;type=cron;keywords=environment,health,check,interval,frequency,heartbeat,status,monitoring,uptime,jobs,schedule;description=How often to check environment connectivity (cron expression)" catmeta:"id=jobschedule;title=Automations;icon=jobs;url=/settings/jobs;description=Configure how often Arcane background jobs run"`

	// Docker category
	AutoUpdate                     SettingVariable `key:"autoUpdate" meta:"label=Auto Update;type=boolean;keywords=auto,update,automatic,upgrade,refresh,restart,deploy;category=internal;description=Automatically update containers when new images are available"`
	AutoUpdateInterval             SettingVariable `key:"autoUpdateInterval" meta:"label=Auto Update Interval;type=cron;keywords=auto,update,interval,frequency,schedule,automatic,timing;category=internal;description=How often to check for automatic updates (cron expression)"`
	AutoUpdateExcludedContainers   SettingVariable `key:"autoUpdateExcludedContainers" meta:"label=Excluded Containers;type=text;keywords=exclude,containers,ignore,skip;category=internal;description=Comma-separated list of containers to exclude from auto-update"`
	PollingEnabled                 SettingVariable `key:"pollingEnabled" meta:"label=Enable Polling;type=boolean;keywords=polling,check,monitor,watch,scan,detection,automatic;category=internal;description=Enable automatic checking for image updates"`
	PollingInterval                SettingVariable `key:"pollingInterval" meta:"label=Polling Interval;type=cron;keywords=interval,frequency,schedule,time,minutes,period,delay;category=internal;description=How often to check for image updates (cron expression)"`
	ImageEventWatcherEnabled       SettingVariable `key:"imageEventWatcherEnabled" meta:"label=Enable Event-Driven Image Checks;type=boolean;keywords=image,updates,event,watcher,registry,rate,limit;category=internal;description=Check image registries immediately after Docker image events; registry rate limits may apply"`
	DockerClientRefreshInterval    SettingVariable `key:"dockerClientRefreshInterval" meta:"label=Docker Client Refresh Interval;type=cron;keywords=docker,client,refresh,daemon,api,version,reconnect,renegotiate,schedule;category=internal;description=How often to refresh the cached Docker client API version (cron expression)"`
	EventCleanupInterval           SettingVariable `key:"eventCleanupInterval" meta:"label=Event Cleanup Interval;type=cron;keywords=events,cleanup,retention,interval,frequency,schedule,history,logs,jobs;description=How often to trim event and activity history to its count limits (cron expression)"`
	ExpiredSessionsCleanupInterval SettingVariable `key:"expiredSessionsCleanupInterval" meta:"label=Expired Sessions Cleanup Interval;type=cron;keywords=sessions,cleanup,retention,expired,revoked,interval,frequency,schedule,auth,jobs;description=How often to delete expired and old revoked sessions (cron expression)"`
	UpgradeLogRetentionDays        SettingVariable `key:"upgradeLogRetentionDays" meta:"label=Upgrade Log Retention;type=number;keywords=upgrade,logs,retention,days,cleanup;category=activity;description=Delete upgrade log files older than this many days. Cleanup runs hourly. Default: 3 days. Set 0 to keep logs indefinitely."`
	MaxConcurrentActivities        SettingVariable `key:"maxConcurrentActivities" meta:"label=Concurrent Activity Limit;type=number;keywords=activity,concurrency,limit,queue,parallel,bulk,background,tasks;category=activity;description=Maximum long-running activities per environment before new ones queue. Set 0 for unlimited."`
	KeepActionOutput               SettingVariable `key:"keepActionOutput" meta:"label=Keep Action Output;type=boolean;category=activity;description=Configured action output retention"`
	ActionPresentation             SettingVariable `key:"actionPresentation" meta:"label=Action Presentation;type=select;category=activity;description=Browser presentation of important actions"`
	AutoInjectEnv                  SettingVariable `key:"autoInjectEnv" meta:"label=Auto Inject Env Variables;type=boolean;keywords=auto,inject,env,environment,variables,interpolation;category=internal;description=Automatically inject project .env variables into all containers (default: false)"`
	DefaultDeployPullPolicy        SettingVariable `key:"defaultDeployPullPolicy" meta:"label=Default Deploy Pull Policy;type=select;keywords=deploy,pull,policy,compose,up,missing,always;category=internal;description=Default image pull policy when deploying projects"`
	ToolsImage                     SettingVariable `key:"toolsImage,envOverride" meta:"label=Tools Image;type=text;keywords=tools,image,registry,helper,volume;category=internal;description=Complete image reference for the Docklet tools helper"`
	UpdateCheckRegistry            SettingVariable `key:"updateCheckRegistry,envOverride" meta:"label=Update Check Registry;type=select;keywords=update,check,registry,ghcr,docker,hub,ecr,self,image,digest;category=internal;description=Registry queried when checking whether the Arcane image has a newer digest"`
	ScheduledPruneEnabled          SettingVariable `key:"scheduledPruneEnabled" meta:"label=Scheduled Prune Enabled;type=boolean;keywords=prune,cleanup,maintenance,schedule,automatic;category=internal;description=Enable scheduled pruning of unused Docker resources"`
	ScheduledPruneInterval         SettingVariable `key:"scheduledPruneInterval" meta:"label=Scheduled Prune Interval;type=cron;keywords=prune,cleanup,interval,minutes,schedule;category=internal;description=How often to run scheduled prunes (cron expression)"`
	PruneContainerMode             SettingVariable `key:"pruneContainerMode" meta:"label=Prune Containers;type=select;keywords=prune,containers,cleanup,maintenance,mode,older,stopped;category=internal;description=Select how containers should be pruned when the scheduled prune job runs"`
	PruneContainerUntil            SettingVariable `key:"pruneContainerUntil" meta:"label=Container Age Filter;type=text;keywords=prune,containers,cleanup,maintenance,until,older,duration;category=internal;description=Duration threshold for scheduled container prune when mode is olderThan"`
	PruneImageMode                 SettingVariable `key:"pruneImageMode" meta:"label=Prune Images;type=select;keywords=prune,images,cleanup,maintenance,mode,dangling,all,older;category=internal;description=Select how images should be pruned when the scheduled prune job runs"`
	PruneImageUntil                SettingVariable `key:"pruneImageUntil" meta:"label=Image Age Filter;type=text;keywords=prune,images,cleanup,maintenance,until,older,duration;category=internal;description=Duration threshold for scheduled image prune when mode is olderThan"`
	PruneVolumeMode                SettingVariable `key:"pruneVolumeMode" meta:"label=Prune Volumes;type=select;keywords=prune,volumes,cleanup,maintenance,mode,anonymous,named;category=internal;description=Select how volumes should be pruned when the scheduled prune job runs"`
	PruneNetworkMode               SettingVariable `key:"pruneNetworkMode" meta:"label=Prune Networks;type=select;keywords=prune,networks,cleanup,maintenance,mode,unused,older;category=internal;description=Select how networks should be pruned when the scheduled prune job runs"`
	PruneNetworkUntil              SettingVariable `key:"pruneNetworkUntil" meta:"label=Network Age Filter;type=text;keywords=prune,networks,cleanup,maintenance,until,older,duration;category=internal;description=Duration threshold for scheduled network prune when mode is olderThan"`
	PruneBuildCacheMode            SettingVariable `key:"pruneBuildCacheMode" meta:"label=Prune Build Cache;type=select;keywords=prune,build cache,cleanup,maintenance,mode,unused,all,older;category=internal;description=Select how build cache should be pruned when the scheduled prune job runs"`
	PruneBuildCacheUntil           SettingVariable `key:"pruneBuildCacheUntil" meta:"label=Build Cache Age Filter;type=text;keywords=prune,build cache,cleanup,maintenance,until,older,duration;category=internal;description=Duration threshold for scheduled build cache prune when mode is olderThan"`
	AutoHealEnabled                SettingVariable `key:"autoHealEnabled" meta:"label=Auto Heal;type=boolean;keywords=auto,heal,health,restart,unhealthy,recovery,container,healthcheck;category=internal;description=Automatically restart containers that become unhealthy"`
	AutoHealInterval               SettingVariable `key:"autoHealInterval" meta:"label=Auto Heal Interval;type=cron;keywords=auto,heal,interval,frequency,schedule,health,jobs;description=How often to check container health (cron expression)" catmeta:"id=jobschedule"`
	AutoHealExcludedContainers     SettingVariable `key:"autoHealExcludedContainers" meta:"label=Auto Heal Excluded Containers;type=text;keywords=auto,heal,exclude,containers,ignore,skip,health;category=internal;description=Comma-separated list of containers to exclude from auto-heal"`
	AutoHealMaxRestarts            SettingVariable `key:"autoHealMaxRestarts" meta:"label=Auto Heal Max Restarts;type=number;keywords=auto,heal,max,restarts,limit,loop,protection;category=internal;description=Maximum auto-heal restarts per container within the restart window (default: 5)"`
	AutoHealRestartWindow          SettingVariable `key:"autoHealRestartWindow" meta:"label=Auto Heal Restart Window;type=number;keywords=auto,heal,restart,window,minutes,cooldown,protection;category=internal;description=Time window in minutes for counting auto-heal restarts (default: 30)"`
	VolumeHelperIdleTimeout        SettingVariable `key:"volumeHelperIdleTimeout" meta:"label=Volume Helper Idle Timeout;type=number;keywords=volume,workspace,helper,idle,timeout,cleanup,reaper,minutes;category=internal;description=Minutes a volume helper container may sit idle before automatic removal (default: 10; 0 disables)"`
	MaxImageUploadSize             SettingVariable `key:"maxImageUploadSize" meta:"label=Max Image Upload Size;type=number;keywords=upload,size,limit,maximum,image,tar,file,megabytes,mb,storage;category=internal;description=Maximum size in MB for image archive uploads (default: 500)"`
	DockerHost                     SettingVariable `key:"dockerHost,authrequired,envOverride" meta:"label=Docker Host;type=text;keywords=docker,host,daemon,socket,unix,remote;category=internal;description=URI for Docker daemon"`
	BuildProvider                  SettingVariable `key:"buildProvider,envOverride" meta:"label=Build Provider;type=select;keywords=build,buildkit,depot,provider,remote,local;category=build;description=Default build provider (local or depot)" catmeta:"id=build;title=Build;icon=code;url=/settings/builds;description=Configure BuildKit and Depot build settings"`
	BuildTimeout                   SettingVariable `key:"buildTimeout,envOverride" meta:"label=Build Timeout;type=number;keywords=build,timeout,seconds,buildkit;category=build;description=Timeout for BuildKit builds in seconds (default: 1800 = 30 minutes)"`
	DepotProjectId                 SettingVariable `key:"depotProjectId,envOverride" meta:"label=Depot Project ID;type=text;keywords=depot,project,id,build,provider;category=build;description=Depot project identifier"`
	DepotToken                     SettingVariable `key:"depotToken,envOverride,sensitive" meta:"label=Depot Token;type=password;keywords=depot,token,api,secret,build,provider;category=build;description=Depot API token"`

	// Authentication and security categories
	AuthLocalEnabled                      SettingVariable `key:"authLocalEnabled,public" meta:"label=Local Authentication;type=boolean;keywords=local,auth,authentication,username,password,login,credentials;category=authentication;description=Enable local username/password authentication" catmeta:"id=authentication;title=Authentication;icon=lock;url=/settings/authentication;description=Manage authentication providers, password policy, and session behavior"`
	AuthSessionTimeout                    SettingVariable `key:"authSessionTimeout" meta:"label=Session Timeout;type=number;keywords=session,timeout,expire,duration,lifetime,minutes,logout;category=authentication;description=How long user sessions remain active"`
	AuthPasswordPolicy                    SettingVariable `key:"authPasswordPolicy" meta:"label=Password Policy;type=select;keywords=password,policy,strength,complexity,requirements,security,rules;category=authentication;description=Set password strength requirements"`
	FeatureVulnerabilityManagementEnabled SettingVariable `key:"featureVulnerabilityManagementEnabled,public,envOverride" meta:"label=Vulnerability management;type=boolean;keywords=features,vulnerability,security,scan,enable,disable;category=general;description=Enable vulnerability management for this environment. Disabling retains reports and configuration and leaves standalone image patching available"`
	VulnerabilityScanEnabled              SettingVariable `key:"vulnerabilityScanEnabled" meta:"label=Scheduled Vulnerability Scan;type=boolean;keywords=vulnerability,scan,security,trivy,schedule,automatic,cve;category=security;description=Enable scheduled vulnerability scanning of all Docker images" catmeta:"id=security;title=Security;icon=shield;url=/settings/security;description=Configure vulnerability scanning and runtime security settings"`
	VulnerabilityScanInterval             SettingVariable `key:"vulnerabilityScanInterval" meta:"label=Vulnerability Scan Interval;type=cron;keywords=vulnerability,scan,interval,schedule,frequency,trivy,cve;category=security;description=How often to run scheduled vulnerability scans (cron expression)"`
	VulnerabilityThreatIntelEnabled       SettingVariable `key:"vulnerabilityThreatIntelEnabled,envOverride" meta:"label=Threat Intelligence;type=boolean;keywords=kev,cisa,epss,first,exploit,threat,intel,risk,airgap,offline,security;category=security;description=Fetch the CISA KEV catalog and FIRST EPSS scores to prioritize exploited vulnerabilities. Disable for air-gapped installs."`
	TrivyDbRegistry                       SettingVariable `key:"trivyDbRegistry,envOverride" meta:"label=Trivy DB Registry;type=text;keywords=trivy,db,java-db,checks,registry,ghcr,docker,hub,vulnerability,security,scanner;category=security;description=Container registry used to pull the Trivy vulnerability database artifacts"`
	TrivyNetwork                          SettingVariable `key:"trivyNetwork,envOverride" meta:"label=Trivy Network;type=text;keywords=trivy,network,mode,bridge,host,none,scanner,vulnerability,security;category=security;description=Docker network mode/network name used for Trivy scan containers. Leave empty to inherit Arcane's network automatically."`
	TrivySecurityOpts                     SettingVariable `key:"trivySecurityOpts,envOverride" meta:"label=Trivy Security Options;type=textarea;keywords=trivy,security,opt,security_opt,selinux,labels,apparmor,scanner;category=security;description=Docker security options applied to Trivy scan containers. Use commas or new lines to separate entries (for example: label=disable)"`
	TrivyPrivileged                       SettingVariable `key:"trivyPrivileged,envOverride" meta:"label=Trivy Privileged;type=boolean;keywords=trivy,privileged,security,selinux,scanner;category=security;description=Run Trivy scan containers in privileged mode when required by the host security policy"`
	TrivyResourceLimitsEnabled            SettingVariable `key:"trivyResourceLimitsEnabled,envOverride" meta:"label=Trivy Resource Limits;type=boolean;keywords=trivy,resources,limits,cpu,memory,ram,security,scan;category=security;description=Enable CPU and memory limits for Trivy scan containers"`
	TrivyCpuLimit                         SettingVariable `key:"trivyCpuLimit,envOverride" meta:"label=Trivy CPU Limit (cores);type=number;keywords=trivy,cpu,cores,limit,scanner,resources;category=security;description=Maximum CPU cores for Trivy scan containers (supports decimals, e.g. 1.5). Set 0 to disable CPU limit"`
	TrivyMemoryLimitMb                    SettingVariable `key:"trivyMemoryLimitMb,envOverride" meta:"label=Trivy Memory Limit (MB);type=number;keywords=trivy,memory,ram,mb,limit,scanner,resources;category=security;description=Maximum memory for Trivy scan containers in MB. Set 0 to disable memory limit"`
	TrivyConcurrentScanContainers         SettingVariable `key:"trivyConcurrentScanContainers" meta:"label=Trivy Concurrent Scan Containers;type=number;keywords=trivy,concurrent,scan,containers,parallel,workers,limit,security;category=security;description=Maximum number of concurrent Trivy scan containers for manual and scheduled scans. Minimum 1"`
	TrivyConfig                           SettingVariable `key:"trivyConfig" meta:"label=Trivy Config (YAML);type=textarea;keywords=trivy,config,yaml,configuration,scanner,settings;category=security;description=Trivy configuration file content in YAML format"`
	TrivyIgnore                           SettingVariable `key:"trivyIgnore" meta:"label=.trivyignore;type=textarea;keywords=trivy,ignore,ignorefile,vulnerabilities,exceptions,exclusions;category=security;description=Trivy ignore file content - one vulnerability ID per line"`
	TrivyServerEnabled                    SettingVariable `key:"trivyServerEnabled" meta:"label=Trivy Client/Server Mode;type=boolean;keywords=trivy,server,client,remote,scanner,vulnerability,security,armv7,32bit;category=security;description=Scan against a remote Trivy server instead of downloading the vulnerability database locally. Recommended for 32-bit hosts (arm/v7) where the local DB cannot be memory-mapped."`
	TrivyServerUrl                        SettingVariable `key:"trivyServerUrl" meta:"label=Trivy Server URL;type=text;keywords=trivy,server,url,remote,scanner,vulnerability,security;category=security;description=URL of the remote Trivy server (e.g. http://trivy.example.com:4954). Used when client/server mode is enabled."`
	TrivyServerToken                      SettingVariable `key:"trivyServerToken,sensitive" meta:"label=Trivy Server Token;type=password;keywords=trivy,server,token,auth,remote,scanner,security;category=security;description=Optional authentication token sent to the remote Trivy server. Leave empty if the server requires no token."`
	TrivyIgnoreUnfixed                    SettingVariable `key:"trivyIgnoreUnfixed,envOverride" meta:"label=Only Report Fixable Vulnerabilities;type=boolean;keywords=trivy,fixable,unfixed,ignore,vulnerability,security,noise,fixes;category=security;description=Only report vulnerabilities that have a known fix available. Reduces noise from vulnerabilities you cannot act on."`
	ImagePatchSuffix                      SettingVariable `key:"imagePatchSuffix" meta:"label=Patched Image Tag Suffix;type=text;keywords=patch,copa,copacetic,image,tag,suffix,vulnerability,security;category=security;description=Suffix appended to the source tag when patching an image (default: patched)"`
	ImagePatchTimeoutSec                  SettingVariable `key:"imagePatchTimeoutSec,envOverride" meta:"label=Image Patch Timeout (seconds);type=number;keywords=patch,copa,copacetic,image,timeout,seconds,buildkit,security;category=security;description=Timeout for a single image patch operation in seconds (default: 600)"`
	ImagePatchAllPlatforms                SettingVariable `key:"imagePatchAllPlatforms" meta:"label=Patch All Platforms;type=boolean;keywords=patch,copa,copacetic,image,platform,architecture,multi,arch,security;category=security;description=Patch every platform in a multi-platform image instead of only the platform this server runs on"`
	ImageAutoPatchEnabled                 SettingVariable `key:"imageAutoPatchEnabled" meta:"label=Scheduled Image Patching;type=boolean;keywords=patch,copa,copacetic,image,automatic,schedule,vulnerability,security,cve;category=security;description=Automatically patch images whose latest vulnerability scan found fixable OS package vulnerabilities"`
	ImageAutoPatchInterval                SettingVariable `key:"imageAutoPatchInterval" meta:"label=Image Patch Interval;type=cron;keywords=patch,copa,copacetic,image,interval,schedule,frequency,cron,security;category=security;description=How often to run scheduled image patching (cron expression)"`
	LifecycleMaxTimeoutSec                SettingVariable `key:"lifecycleMaxTimeoutSec,envOverride" meta:"label=Lifecycle Hook Max Timeout (seconds);type=number;keywords=lifecycle,hook,timeout,seconds,limit,maximum,script,pre-deploy;category=security;description=Maximum allowed timeout for lifecycle scripts in seconds (default: 300)"`

	// Notifications category (placeholder for category metadata only - actual settings managed via notification service)
	NotificationsCategoryPlaceholder SettingVariable `key:"notificationsCategory,internal" meta:"label=Notifications;type=internal;keywords=notifications,alerts,email,discord,webhooks,events,messages;category=notifications;description=Configure notification providers and alerts" catmeta:"id=notifications;title=Notifications;icon=bell;url=/settings/notifications;description=Configure email and Discord notifications for container and image updates"`

	AgentToken               SettingVariable `key:"agentToken,internal,sensitive"`
	BrowserSessionSigningKey SettingVariable `key:"browserSessionSigningKey,internal,sensitive"`
	JwtSigningKeySeed        SettingVariable `key:"jwtSigningKeySeed,internal,sensitive"`
	InstanceID               SettingVariable `key:"instanceId,internal"`
	AuthLastMode             SettingVariable `key:"authLastMode,internal"`

	// Users category (admin management page - no actual settings)
	UsersCategoryPlaceholder SettingVariable `key:"usersCategory,internal" meta:"label=Users;type=internal;keywords=users,accounts,management,admin,access,permissions,roles;category=users;description=Manage user accounts and permissions" catmeta:"id=users;title=Users;icon=user;url=/settings/users;description=Manage user accounts and access control"`

	// API Keys category (admin management page - no actual settings)
	ApiKeysCategoryPlaceholder SettingVariable `key:"apiKeysCategory,internal" meta:"label=API Keys;type=internal;keywords=api,keys,tokens,authentication,access,programmatic,integration;category=apikeys;description=Manage API keys for programmatic access" catmeta:"id=apikeys;title=API Keys;icon=apikey;url=/settings/api-keys;description=Create and manage API keys for programmatic access to Arcane"`

	SystemBackupsCategoryPlaceholder SettingVariable `key:"systemBackupsCategory,internal" meta:"label=Backups;type=internal;keywords=backup,recovery,restore,configuration,secrets;category=systembackups;description=Back up and restore this Arcane instance" catmeta:"id=systembackups;title=Backups;icon=backup;url=/settings/backups;description=Create encrypted recovery backups of Arcane data and configuration"`
	SystemVolumeBackupConfig         SettingVariable `key:"systemVolumeBackupConfig,internal"`

	// Webhooks category (management page - no actual settings)
	WebhooksCategoryPlaceholder SettingVariable `key:"webhooksCategory,internal" meta:"label=Webhooks;type=internal;keywords=webhooks,trigger,inbound,http,container,stack,updater,automation,ci,cd;category=webhooks;description=Manage inbound webhooks to trigger updates" catmeta:"id=webhooks;title=Webhooks;icon=globe;url=/settings/webhooks;description=Create and manage inbound webhooks to trigger container, stack updates"`

	// Timeout category
	DockerAPITimeout       SettingVariable `key:"dockerApiTimeout,envOverride" meta:"label=Docker API Timeout;type=number;keywords=docker,api,timeout,seconds,list,operations;category=timeouts;description=Timeout for Docker list operations in seconds (default: 30)" catmeta:"id=timeouts;title=Timeouts;icon=clock;url=/settings/timeouts;description=Configure operation timeouts for slow networks or hardware"`
	DockerImagePullTimeout SettingVariable `key:"dockerImagePullTimeout,envOverride" meta:"label=Docker Image Pull Timeout;type=number;keywords=docker,image,pull,timeout,seconds,download;category=timeouts;description=Timeout for Docker image pulls in seconds (default: 600 = 10 minutes)"`
	TrivyScanTimeout       SettingVariable `key:"trivyScanTimeout,envOverride" meta:"label=Trivy Scan Timeout;type=number;keywords=trivy,vulnerability,scan,timeout,seconds,cve;category=timeouts;description=Timeout for Trivy image scans in seconds (default: 900 = 15 minutes)"`
	HTTPClientTimeout      SettingVariable `key:"httpClientTimeout,envOverride" meta:"label=HTTP Client Timeout;type=number;keywords=http,client,timeout,seconds,api,request;category=timeouts;description=Default timeout for HTTP requests in seconds (default: 30)"`
	RegistryTimeout        SettingVariable `key:"registryTimeout,envOverride" meta:"label=Registry Timeout;type=number;keywords=registry,timeout,seconds,docker,auth;category=timeouts;description=Timeout for container registry operations in seconds (default: 30)"`
	RegistryTagTimeout     SettingVariable `key:"registryTagTimeout,envOverride" meta:"label=Registry Tag Timeout;type=number;keywords=registry,tags,timeout,seconds,pagination,update,version;category=timeouts;description=Timeout for listing every tag in a repository during version update checks in seconds (default: 120)"`
	ProxyRequestTimeout    SettingVariable `key:"proxyRequestTimeout,envOverride" meta:"label=Proxy Request Timeout;type=number;keywords=proxy,request,timeout,seconds,forward;category=timeouts;description=Timeout for proxied requests in seconds (default: 60)"`
	DeployWaitTimeout      SettingVariable `key:"deployWaitTimeout,envOverride" meta:"label=Deploy Wait Timeout;type=number;keywords=deploy,compose,up,wait,healthy,depends_on,timeout,seconds;category=timeouts;description=Timeout waiting for services to become healthy or complete during a deploy in seconds (default: 600 = 10 minutes)"`
}

func (SettingVariable) TableName() string {
	return "settings"
}

func buildSettingsFieldCacheInternal() {
	rt := reflect.TypeFor[Settings]()
	ordered := make([]settingFieldMeta, 0, rt.NumField())
	byKey := make(map[string]settingFieldMeta, rt.NumField())

	for i := range rt.NumField() {
		field := rt.Field(i)
		key, attrs, _ := strings.Cut(field.Tag.Get("key"), ",")
		if key == "" {
			continue
		}

		attrList := splitSettingAttrsInternal(attrs)

		meta := settingFieldMeta{
			index:               i,
			key:                 key,
			attrs:               attrs,
			isPublic:            slices.Contains(attrList, "public"),
			isVisibleToNonAdmin: slices.Contains(attrList, "public") || slices.Contains(attrList, "authrequired"),
			isSensitive:         slices.Contains(attrList, "sensitive"),
		}
		ordered = append(ordered, meta)
		byKey[key] = meta
	}

	settingsFieldCache.ordered = ordered
	settingsFieldCache.byKey = byKey
}

func getSettingsFieldCacheInternal() ([]settingFieldMeta, map[string]settingFieldMeta) {
	settingsFieldCache.once.Do(buildSettingsFieldCacheInternal)
	return settingsFieldCache.ordered, settingsFieldCache.byKey
}

func splitSettingAttrsInternal(attrs string) []string {
	if attrs == "" {
		return nil
	}

	return strings.Split(attrs, ",")
}

func (s *Settings) Clone() *Settings {
	if s == nil {
		return &Settings{}
	}

	return new(*s)
}

func (s *Settings) ToSettingVariableSlice(visibility SettingVisibility, redactSensitiveValues bool) []SettingVariable {
	cfgValue := reflect.ValueOf(s).Elem()
	fields, _ := getSettingsFieldCacheInternal()

	res := make([]SettingVariable, 0, len(fields))
	for _, field := range fields {
		if !fieldVisibleForSettingVisibilityInternal(field, visibility) {
			continue
		}

		value := cfgValue.Field(field.index).FieldByName("Value").String()
		value = kit.Ternary(value == "" || !redactSensitiveValues || !strings.Contains(field.attrs, "sensitive"), value, redactionMask)

		settingVariable := SettingVariable{
			Key:   field.key,
			Value: value,
		}
		res = append(res, settingVariable)
	}

	return res
}

func fieldVisibleForSettingVisibilityInternal(field settingFieldMeta, visibility SettingVisibility) bool {
	switch visibility {
	case SettingVisibilityPublic:
		return field.isPublic
	case SettingVisibilityNonAdmin:
		return field.isVisibleToNonAdmin
	case SettingVisibilityAll:
		return true
	default:
		return false
	}
}

func (s *Settings) FieldByKey(key string) (defaultValue string, isPublic, isSensitive bool, err error) {
	rv := reflect.ValueOf(s).Elem()
	_, byKey := getSettingsFieldCacheInternal()

	field, ok := byKey[key]
	if !ok {
		return "", false, false, SettingKeyNotFoundError{field: key}
	}

	valueField := rv.Field(field.index).FieldByName("Value")
	return valueField.String(), field.isPublic, field.isSensitive, nil
}

func (s *Settings) UpdateField(key, value string, noSensitive bool) error {
	rv := reflect.ValueOf(s).Elem()
	_, byKey := getSettingsFieldCacheInternal()

	field, ok := byKey[key]
	if !ok {
		return SettingKeyNotFoundError{field: key}
	}

	if noSensitive && field.isSensitive {
		return SettingSensitiveForbiddenError{field: key}
	}

	valueField := rv.Field(field.index).FieldByName("Value")
	if !valueField.CanSet() {
		return fmt.Errorf("field Value in SettingVariable is not settable for config key '%s'", key)
	}

	valueField.SetString(value)
	return nil
}

type SettingKeyNotFoundError struct {
	field string
}

func (e SettingKeyNotFoundError) Error() string {
	return "cannot find setting key '" + e.field + "'"
}

func (e SettingKeyNotFoundError) Is(target error) bool {
	x := SettingKeyNotFoundError{}
	return errors.As(target, &x)
}

type SettingSensitiveForbiddenError struct {
	field string
}

func (e SettingSensitiveForbiddenError) Error() string {
	return "field '" + e.field + "' is sensitive and can't be updated"
}

func (e SettingSensitiveForbiddenError) Is(target error) bool {
	x := SettingSensitiveForbiddenError{}
	return errors.As(target, &x)
}
