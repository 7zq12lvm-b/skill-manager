package skillmgr

import (
	"context"
	"errors"
	"strings"
)

const GitProvider = "git"

type SourceProvider interface {
	Kind() string
	Inspect(context.Context, string) (SourceInstallation, string, error)
}

type GitSourceProvider struct{}

func (GitSourceProvider) Kind() string {
	return GitProvider
}

func (GitSourceProvider) Inspect(ctx context.Context, path string) (SourceInstallation, string, error) {
	gitRoot, ok := gitRepositoryRoot(ctx, path)
	if !ok {
		return SourceInstallation{}, "", errors.New("cross-device sync requires a Git repository")
	}
	remote, ok := gitRemoteURL(ctx, gitRoot)
	if !ok {
		return SourceInstallation{}, "", errors.New("cross-device sync requires a Git repository with a usable remote")
	}
	sourceID, ok := canonicalGitRemote(remote)
	if !ok {
		return SourceInstallation{}, "", errors.New("the Git remote could not be converted to a portable source ID")
	}
	return SourceInstallation{
		Provider: GitProvider,
		SourceID: sourceID,
		Path:     gitRoot,
		Enabled:  true,
		Options: SourceInstallationOptions{
			ScanRoots: []string{"."},
		},
	}, strings.TrimSpace(remote), nil
}

func ProviderFor(kind string) (SourceProvider, bool) {
	switch strings.TrimSpace(kind) {
	case GitProvider:
		return GitSourceProvider{}, true
	default:
		return nil, false
	}
}

func PortableSourceKey(provider, sourceID string) string {
	provider = strings.TrimSpace(provider)
	sourceID = strings.Trim(strings.TrimSpace(sourceID), "/")
	if provider == "" || sourceID == "" {
		return ""
	}
	return provider + ":" + sourceID
}
