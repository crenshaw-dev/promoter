# Getting Started

This guide will help you get started installing and setting up the GitOps Promoter. We currently only support
GitHub and GitHub Enterprise as the SCM providers. We would welcome any contributions to add support for other
providers.

## Requirements

* kubectl CLI
* kubernetes cluster
* GitHub or GitHub Enterprise Application
  * Will take PRs to add support for other SCM providers

## Installation

To install GitOps Promoter, you can use the following command:

```bash
kubectl apply -f https://github.com/argoproj-labs/gitops-promoter/releases/download/v0.0.1-rc3/install.yaml
```

## GitHub App Configuration

You will need to [create a GitHub App](https://docs.github.com/en/developers/apps/creating-a-github-app) and configure
it to allow the GitOps Promoter to interact with your GitHub repository.

!!! note "Configure your webhook ingress"

    We do support configuration of a GitHub App webhook that triggers PR creation upon Push. However, we do not configure
    the ingress to allow GitHub to reach the GitOps Promoter. You will need to configure the ingress to allow GitHub to reach 
    the GitOps Promoter via the service promoter-webhook-receiver which listens on port `3333`. If you do not use webhooks 
    you might want to adjust the auto reconciliation interval to a lower value using these CLI flags `--promotion-strategy-requeue-duration` and
    `--change-transfer-policy-requeue-duration`.

During the creation the GitHub App, you will need to configure the following settings:

* Permissions
  * Commit statuses - Read & write
  * Contents - Read & write
  * Pull requests - Read & write
* Webhook URL (Optional - but highly recommended)
  * `https://<your-promoter-webhook-receiver-service>/`

The GitHub App will generate a private key that you will need to save. You will also need to get the App ID and the
installation ID in a secret as follows:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <your-secret-name>
type: Opaque
stringData:
  appID: <your-app-id>
  installationID: <your-installation-id>
  privateKey: <your-private-key>
```

!!! note 

    This Secret will need to be installed to the same namespace that you plan on creating PromotionStrategy resources in.

We also need a GitRepository and ScmProvider, which is are custom resources that represents a git repository and a provider. 
Here is an example of both resources:

```yaml
apiVersion: promoter.argoproj.io/v1alpha1
kind: ScmProvider
metadata:
  name: <your-scmprovider-name>
spec:
  secretRef:
    name: <your-secret-name>
  github: {}
---
apiVersion: promoter.argoproj.io/v1alpha1
kind: GitRepository
metadata:
  name: <git-repository-ref-name>
spec:
  name: <repo-name>
  owner: <github-org-username>
  scmProviderRef:
    name: <your-scmprovider-name> # The secret that contains the GitHub App configuration
```

!!! note 

    The GitRepository and ScmProvider also need to be installed to the same namespace that you plan on creating PromotionStrategy 
    resources in, and it also needs to be in the same namespace of the secret it references.

## Promotion Strategy

The PromotionStrategy resource is the main resource that you will use to configure the promotion of your application to different environments.
Here is an example PromotionStrategy resource:

```yaml
apiVersion: promoter.argoproj.io/v1alpha1
kind: PromotionStrategy
metadata:
  name: demo
spec:
  environments:
  - autoMerge: false
    branch: environment/development
  - autoMerge: false
    branch: environment/staging
  - autoMerge: false
    branch: environment/production
  gitRepositoryRef:
    name: <git-repository-ref-name> # The name of the GitRepository resource
```

!!! note 

    Notice that the branches are prefixed with `environment/`. This is a convention that we recommend you follow.

!!! note 

    The `autoMerge` field is optional and defaults to `true`. We set it to `false` here because we do not have any
    CommitStatus checks configured. With these all set to `false` we will have to manually merge the PRs.