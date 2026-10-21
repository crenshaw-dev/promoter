package bitbucket_datacenter

import (
	"context"
	"fmt"
	"net/url"

	corev1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/argoproj-labs/gitops-promoter/api/v1alpha1"
	"github.com/argoproj-labs/gitops-promoter/internal/scms"
)

// GitAuthenticationProvider implements the scms.GitOperationsProvider interface for Bitbucket DataCenter/Server.
type GitAuthenticationProvider struct {
	scmProvider v1alpha1.GenericScmProvider
	secret      *corev1.Secret
}

var _ scms.GitOperationsProvider = &GitAuthenticationProvider{}

// NewBitbucketDataCenterGitAuthenticationProvider creates a new instance of GitAuthenticationProvider
// for Bitbucket DataCenter/Server.
func NewBitbucketDataCenterGitAuthenticationProvider(scmProvider v1alpha1.GenericScmProvider, secret *corev1.Secret) (*GitAuthenticationProvider, error) {
	if scmProvider.GetSpec().BitbucketDataCenter == nil {
		return nil, fmt.Errorf("scmProvider %q does not have BitbucketDataCenter configuration", scmProvider.GetName())
	}
	return &GitAuthenticationProvider{
		scmProvider: scmProvider,
		secret:      secret,
	}, nil
}

// GetGitHttpsRepoUrl constructs the HTTPS URL for a Bitbucket DataCenter/Server repository.
func (g GitAuthenticationProvider) GetGitHttpsRepoUrl(repo v1alpha1.GitRepository) string {
	domain := g.scmProvider.GetSpec().BitbucketDataCenter.Domain
	project := repo.Spec.BitbucketDataCenter.Project
	name := repo.Spec.BitbucketDataCenter.Name
	repoURL := fmt.Sprintf("https://%s/scm/%s/%s.git", domain, project, name)
	if _, err := url.Parse(repoURL); err != nil {
		return ""
	}
	return repoURL
}

// GetToken returns the HTTP Basic password. That is the secret password, which may be a user HTTP access token,
// or the project or repository token when the secret has only "token".
func (g GitAuthenticationProvider) GetToken(ctx context.Context) (string, error) {
	_, password, _, err := credentialsFromSecret(g.secret)
	return password, err
}

// GetUser returns the HTTP Basic username. Username and password must be set together. A project or repository
// token has no owning username, so a secret with only "token" uses x-token-auth.
func (g GitAuthenticationProvider) GetUser(ctx context.Context) (string, error) {
	username, _, _, err := credentialsFromSecret(g.secret)
	return username, err
}
