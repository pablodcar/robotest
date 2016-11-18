package ui

import (
	"fmt"
	"regexp"

	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti"
	. "github.com/sclevine/agouti/matchers"
)

type Bandwagon struct {
	page       *agouti.Page
	domainName string
	email      string
	password   string
}

func OpenBandwagon(page *agouti.Page, domainName string, email string, password string) *Bandwagon {
	urlPrefix := fmt.Sprintf("/web/site/%v", domainName)
	r, _ := regexp.Compile("/web/.*")
	url, _ := page.URL()
	url = r.ReplaceAllString(url, urlPrefix)

	Expect(page.Navigate(url)).To(
		Succeed(),
		"should open bandwagon")

	Eventually(page.FindByClass("my-page-btn-submit"), defaultTimeout).Should(
		BeFound(),
		"should wait for bandwagon to load")

	return &Bandwagon{page: page, email: email, password: password, domainName: domainName}
}

func (b *Bandwagon) SubmitForm() {
	page := b.page

	Expect(page.FindByName("email").Fill(b.email)).To(
		Succeed(),
		"should enter email")

	Expect(page.FindByName("password").Fill(b.password)).To(
		Succeed(),
		"should enter password")

	Expect(page.FindByName("passwordConfirmed").Fill(b.password)).To(
		Succeed(),
		"should re-enter password")

	Expect(page.FindByClass("my-page-btn-submit").Click()).To(
		Succeed(),
		"should click submit btn")

	Eventually(page.FindByClass("my-page-section-endpoints"), defaultTimeout).Should(
		BeFound(),
		"should find endpoints")
}

func (b *Bandwagon) GetEndPoints() []string {
	var result []string
	js := ` var result = []; var cssSelector = ".my-page-section-endpoints-item a"; var children = document.querySelectorAll(cssSelector); children.forEach( z => result.push(z.text) ); return result; `
	b.page.RunScript(js, nil, &result)
	return result
}
