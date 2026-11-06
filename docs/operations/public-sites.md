# Public sites

Diffluence writes public sites as plain files. It never serves them: a web server of your own does (ADR-010).

## Set-up

1. Set `DIFFLUENCE_PUBLISH_DIR` to a folder that both the API and the worker can write to: a withdrawal happens in the API.
2. Turn on public sites and public links under Administration, Export and publishing.
3. Point a web server at `<folder>/<site>/live` for a site, and at `<folder>/links` for public links.

`live` is a link to the current build, changed in one atomic step. The folder name of a site is on its page in Space settings.

## Headers

Every page carries a Content Security Policy in a meta tag, and loads nothing from elsewhere. A web server can do more, which is worth it (SEC-382). For nginx:

```nginx
add_header Content-Security-Policy "default-src 'none'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'" always;
add_header X-Content-Type-Options nosniff always;
add_header Referrer-Policy no-referrer always;
```

For Caddy:

```caddy
header {
    Content-Security-Policy "default-src 'none'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
    X-Content-Type-Options nosniff
    Referrer-Policy no-referrer
}
```

Keep the folder out of the reach of the instance's own address, and do not list its contents: the address of a public link is what protects it.

## PDF

PDF needs the helper process: `docker compose --profile pdf up -d`, and `DIFFLUENCE_PDF_URL=http://pdf:3000`. It gets only HTML that Diffluence made, and needs no outside network. The same helper opens office files as a PDF, on request of someone who may see them. For those it gets the content and the extension of the file. To use Collabora for them instead, set `DIFFLUENCE_COLLABORA_URL`.
