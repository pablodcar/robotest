package site

import (
	"fmt"

	"github.com/gravitational/robotest/e2e/framework"
	ui "github.com/gravitational/robotest/e2e/model/ui"
	"github.com/gravitational/robotest/e2e/model/ui/defaults"

	. "github.com/onsi/gomega"
	web "github.com/sclevine/agouti"
	. "github.com/sclevine/agouti/matchers"
)

type Site struct {
	domainName string
	page       *web.Page
}

func Open(page *web.Page, domainName string) Site {
	site := Site{page: page, domainName: domainName}
	newUrl := site.formatUrl("")
	site.assertSiteNavigation(newUrl)
	return site
}

func (s *Site) GetSiteAppPage() SiteAppPage {
	return SiteAppPage{page: s.page}
}

func (s *Site) GetSiteServerPage() SiteServerPage {
	return SiteServerPage{page: s.page}
}

func (s *Site) NavigateToSiteApp() {
	newUrl := s.formatUrl("")
	s.assertSiteNavigation(newUrl)
}

func (s *Site) NavigateToServers() {
	newUrl := s.formatUrl("servers")
	s.assertSiteNavigation(newUrl)

	Eventually(func() bool {
		count, _ := s.page.All(".grv-site-servers .grv-table td").Count()
		return count > 0
	}, defaults.ServerLoadTimeout).Should(
		BeTrue(),
		"waiting for servers to load")

	ui.PauseForPageJs()
}

func (s *Site) assertSiteNavigation(URL string) {
	Expect(s.page.Navigate(URL)).To(Succeed())
	Eventually(s.page.FindByClass("grv-site"), defaults.ElementTimeout).Should(BeFound(), "waiting for site to be ready")
	ui.PauseForComponentJs()
}

func (s *Site) formatUrl(newPrefix string) string {
	urlPrefix := fmt.Sprintf("/web/site/%v/%v", s.domainName, newPrefix)
	url, err := s.page.URL()
	Expect(err).NotTo(HaveOccurred())
	return framework.URLPathFromString(url, urlPrefix)
}