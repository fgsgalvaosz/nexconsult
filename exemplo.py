test.describe("consulta sintegra - ma", () => {
  test("tests consulta sintegra - ma", async ({ page }) => {
    await page.setViewportSize({
          width: 1115,
          height: 639
        })
    await page.goto("https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraFiltro.jsf");
    await page.locator("#form1\\:tipoEmissao\\:1").click()
    await page.locator("#form1\\:cpfCnpj").click()
    await page.locator("#form1\\:cpfCnpj").type("38139407000177");
    await page.locator("div.recaptcha-checkbox-border").click()
    await page.locator("tr:nth-of-type(3) > td:nth-of-type(2) img").click()
    await page.locator("tr:nth-of-type(2) > td:nth-of-type(2) img").click()
    await page.locator("tr:nth-of-type(3) > td:nth-of-type(1) img").click()
    await page.locator("tr:nth-of-type(2) > td:nth-of-type(1) img").click()
    await page.locator("tr:nth-of-type(1) > td:nth-of-type(2) img").click()
    await page.locator("tr:nth-of-type(1) > td:nth-of-type(1) img").click()
    await page.locator("tr:nth-of-type(4) > td:nth-of-type(2) img").click()
    await page.locator("#recaptcha-verify-button").click()
    await page.locator("tr:nth-of-type(3) > td:nth-of-type(3) img").click()
    await page.locator("tr:nth-of-type(3) > td:nth-of-type(4) img").click()
    await page.locator("tr:nth-of-type(2) > td:nth-of-type(3) img").click()
    await page.locator("tr:nth-of-type(3) > td:nth-of-type(2) img").click()
    await page.locator("#recaptcha-verify-button").click()
    await page.locator("tr:nth-of-type(1) > td:nth-of-type(2) img").click()
    await page.locator("tr:nth-of-type(1) > td:nth-of-type(1) img").click()
    await page.locator("tr:nth-of-type(3) > td:nth-of-type(3) img").click()
    await page.locator("tr:nth-of-type(3) > td:nth-of-type(2) img").click()
    await page.locator("#recaptcha-verify-button").click()
    await page.locator("#form1\\:pnlPrincipal4 input:nth-of-type(2)").click()
    expect(page.url()).toBe('https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraResultadoListaConsulta.jsf');
    await page.locator("#j_id6\\:pnlCadastro img").click()
    expect(page.url()).toBe('https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraResultadoConsulta.jsf');
  });
});
