package bitbucket_datacenter_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/argoproj-labs/gitops-promoter/api/v1alpha1"
	bitbucket_datacenter "github.com/argoproj-labs/gitops-promoter/internal/scms/bitbucket_datacenter"
)

var _ = Describe("GitAuthenticationProvider", func() {
	newProvider := func(data map[string][]byte) *bitbucket_datacenter.GitAuthenticationProvider {
		scmProvider := &v1alpha1.ScmProvider{Spec: v1alpha1.ScmProviderSpec{
			BitbucketDataCenter: &v1alpha1.BitbucketDataCenter{Domain: "bitbucket.example.com"},
		}}
		provider, err := bitbucket_datacenter.NewBitbucketDataCenterGitAuthenticationProvider(scmProvider, &corev1.Secret{Data: data})
		Expect(err).NotTo(HaveOccurred())
		return provider
	}

	Describe("git basic auth", func() {
		It("uses x-token-auth and the token for a project or repository token", func() {
			provider := newProvider(map[string][]byte{"token": []byte("s3cret")})
			user, err := provider.GetUser(context.Background())
			Expect(err).NotTo(HaveOccurred())
			token, err := provider.GetToken(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(user).To(Equal("x-token-auth"))
			Expect(token).To(Equal("s3cret"))
		})

		It("uses the secret username and password, including a user token stored as the password", func() {
			provider := newProvider(map[string][]byte{
				"username": []byte("alice"),
				"password": []byte("pw"),
			})
			user, err := provider.GetUser(context.Background())
			Expect(err).NotTo(HaveOccurred())
			token, err := provider.GetToken(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(user).To(Equal("alice"))
			Expect(token).To(Equal("pw"))
		})

		It("rejects a username paired with a token", func() {
			provider := newProvider(map[string][]byte{
				"username": []byte("alice"),
				"token":    []byte("s3cret"),
			})
			_, err := provider.GetUser(context.Background())
			Expect(err).To(HaveOccurred())
			_, err = provider.GetToken(context.Background())
			Expect(err).To(HaveOccurred())
		})
	})
})
