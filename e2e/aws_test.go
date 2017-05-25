package e2e

import (
	"github.com/gravitational/robotest/e2e/framework"

	"github.com/gravitational/robotest/e2e/uimodel"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = framework.RoboDescribe("AWS Integration Test", func() {
	f := framework.New()
	ctx := framework.TestContext

	var ui uimodel.UI

	BeforeEach(func() {
		ui = uimodel.Init(f.Page)
	})

	It("should provision a new cluster [provisioner:aws_install]", func() {
		domainName := ctx.ClusterName
		ui.EnsureUser(framework.InstallerURL())
		installer := ui.GoToInstaller(framework.InstallerURL())

		By("filling out license text field if required")
		installer.ProcessLicenseStepIfRequired(ctx.License)
		installer.InitAWSInstallation(domainName)

		By("selecting a flavor")
		installer.SelectFlavorByLabel(ctx.FlavorLabel)
		profiles := installer.GetAWSProfiles()
		Expect(len(profiles)).To(BeNumerically(">", 0), "expect at least 1 profile")

		By("setting up AWS instance types")
		for _, p := range profiles {
			p.SetInstanceType(ctx.AWS.InstanceType)
		}

		By("starting an installation")
		installer.StartInstallation()

		By("waiting until install is completed or failed")
		installer.WaitForCompletion()

		if installer.NeedsBandwagon(domainName) == true {
			By("navigating to bandwagon step")
			bandwagon := ui.GoToBandwagon(domainName)
			By("submitting bandwagon form")
			ctx.Bandwagon.RemoteAccess = ctx.ForceRemoteAccess || !ctx.Wizard
			bandwagon.SubmitForm(ctx.Bandwagon)

			By("navigating to a site and reading endpoints")
			site := ui.GoToSite(domainName)
			endpoints := site.GetEndpoints()
			Expect(len(endpoints)).To(BeNumerically(">", 0), "expected at least one application endpoint")
		} else {
			By("clicking on continue")
			installer.ProceedToSite()
		}
	})

	It("should add and remove a server [provisioner:aws_expand_shrink]", func() {
		ui.EnsureUser(framework.SiteURL())
		site := ui.GoToSite(ctx.ClusterName)
		siteServerPage := site.GoToServers()
		newServer := siteServerPage.AddAWSServer()
		siteServerPage.DeleteServer(newServer)
	})

	It("should delete site [provisioner:aws_delete]", func() {
		ui.EnsureUser(framework.SiteURL())
		By("openning opscenter")
		opscenter := ui.GoToOpsCenter(framework.Cluster.OpsCenterURL())
		By("trying to delete a site")
		opscenter.DeleteSite(ctx.ClusterName)
	})
})
