package deploy

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/sentry"
)

const (
	defaultManifestPath = "fly_deploy_manifest.json"
)

type DeployManifest struct {
	AppName               string
	Config                *appconfig.Config         `json:"config"`
	DeploymentImage       string                    `json:"deployment_image,omitempty"`
	Strategy              string                    `json:"strategy,omitempty"`
	EnvFromFlags          []string                  `json:"env_from_flags,omitempty"`
	PrimaryRegionFlag     string                    `json:"primary_region_flag,omitempty"`
	SkipSmokeChecks       bool                      `json:"skip_smoke_checks,omitempty"`
	SkipHealthChecks      bool                      `json:"skip_health_checks,omitempty"`
	SkipDNSChecks         bool                      `json:"skip_dns_checks,omitempty"`
	SkipReleaseCommand    bool                      `json:"skip_release_command,omitempty"`
	MaxUnavailable        *float64                  `json:"max_unavailable,omitempty"`
	RestartOnly           bool                      `json:"restart_only,omitempty"`
	WaitTimeout           *time.Duration            `json:"wait_timeout,omitempty"`
	StopSignal            string                    `json:"stop_signal,omitempty"`
	LeaseTimeout          *time.Duration            `json:"lease_timeout,omitempty"`
	ReleaseCmdTimeout     *time.Duration            `json:"release_cmd_timeout,omitempty"`
	Guest                 *fly.MachineGuest         `json:"guest,omitempty"`
	IncreasedAvailability bool                      `json:"increased_availability,omitempty"`
	AllocPublicIP         bool                      `json:"alloc_public_ip,omitempty"`
	UpdateOnly            bool                      `json:"update_only,omitempty"`
	Files                 []*fly.File               `json:"files,omitempty"`
	ExcludeRegions        map[string]bool           `json:"exclude_regions,omitempty"`
	OnlyRegions           map[string]bool           `json:"only_regions,omitempty"`
	ExcludeMachines       map[string]bool           `json:"exclude_machines,omitempty"`
	OnlyMachines          map[string]bool           `json:"only_machines,omitempty"`
	ProcessGroups         map[string]bool           `json:"process_groups,omitempty"`
	MaxConcurrent         int                       `json:"max_concurrent,omitempty"`
	VolumeInitialSize     int                       `json:"volume_initial_size,omitempty"`
	RestartPolicy         *fly.MachineRestartPolicy `json:"restart_policy,omitempty"`
	RestartMaxRetries     int                       `json:"restart_max_retrie,omitempty"`
	DeployRetries         int                       `json:"deploy_retries,omitempty"`
}

func NewManifest(AppName string, config *appconfig.Config, args MachineDeploymentArgs) *DeployManifest {
	return &DeployManifest{
		AppName:               AppName,
		Config:                config,
		DeploymentImage:       args.DeploymentImage,
		Strategy:              args.Strategy,
		EnvFromFlags:          args.EnvFromFlags,
		PrimaryRegionFlag:     args.PrimaryRegionFlag,
		SkipSmokeChecks:       args.SkipSmokeChecks,
		SkipHealthChecks:      args.SkipHealthChecks,
		SkipDNSChecks:         args.SkipDNSChecks,
		SkipReleaseCommand:    args.SkipReleaseCommand,
		MaxUnavailable:        args.MaxUnavailable,
		RestartOnly:           args.RestartOnly,
		WaitTimeout:           args.WaitTimeout,
		StopSignal:            args.StopSignal,
		LeaseTimeout:          args.LeaseTimeout,
		ReleaseCmdTimeout:     args.ReleaseCmdTimeout,
		Guest:                 args.Guest,
		IncreasedAvailability: args.IncreasedAvailability,
		UpdateOnly:            args.UpdateOnly,
		Files:                 args.Files,
		ExcludeRegions:        args.ExcludeRegions,
		OnlyRegions:           args.OnlyRegions,
		ExcludeMachines:       args.ExcludeMachines,
		OnlyMachines:          args.OnlyMachines,
		ProcessGroups:         args.ProcessGroups,
		MaxConcurrent:         args.MaxConcurrent,
		VolumeInitialSize:     args.VolumeInitialSize,
		RestartPolicy:         args.RestartPolicy,
		RestartMaxRetries:     args.RestartMaxRetries,
		DeployRetries:         args.DeployRetries,
	}
}

func ManifestFromReader(r io.Reader) (*DeployManifest, error) {
	manifest := &DeployManifest{}
	if err := json.NewDecoder(r).Decode(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func ManifestFromFile(filename string) (*DeployManifest, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ManifestFromReader(file)
}

func ManifestIntoWriter(w io.Writer, manifest *DeployManifest) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(manifest)
}

func ManifestIntoFile(manifest *DeployManifest, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return ManifestIntoWriter(file, manifest)
}

func exportManifest(ctx context.Context, appName string, args MachineDeploymentArgs) error {
	config := appconfig.ConfigFromContext(ctx)

	manifest := NewManifest(appName, config, args)

	return ManifestIntoFile(manifest, defaultManifestPath)
}

func deployFromManifest(ctx context.Context, manifest *DeployManifest) error {
	var (
		client = flyutil.ClientFromContext(ctx)
	)
	app, err := client.GetAppCompact(ctx, manifest.AppName)
	if err != nil {
		return err
	}

	args := MachineDeploymentArgs{}

	md, err := NewMachineDeployment(ctx, args)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(ctx, err, "deploy", app)
		return err
	}

	err = md.DeployMachinesApp(ctx)
	if err != nil {
		sentry.CaptureExceptionWithAppInfo(ctx, err, "deploy", app)
		return err
	}
	return nil
}