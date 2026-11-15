# EndPoll site

Static HTML, CSS, and JavaScript, assembled by Eleventy. No theme or plugins.

## Development

```sh
yarn install --frozen-lockfile
yarn dev
```

Open http://localhost:47011. Eleventy rebuilds and reloads the browser when source files change.

## Editing

- `pages/index.html`: homepage content, feature cards, uptime preview, and structured data.
- `pages/docs/*.html`: documentation content. Keep code examples inside `<pre><code>` unchanged when adjusting indentation.
- `assets/css/main.css`: all styling, including colors, responsive layouts, and guide typography.
- `assets/js/status-demo.js`: uptime preview animation. The initial preview is HTML and works without JavaScript.
- `pages/_includes/base.html`: shared document head, navigation, and footer.
- `pages/_includes/guide.html`: shared documentation navigation and wrapper.
- `pages/_data/site.json`: shared site URL, author, and social image metadata.
- `public/`: files copied to the site root, including icons, the social image, and `robots.txt`.

Each page starts with a small YAML metadata block between `---` lines, followed by ordinary HTML. `title` and `description` supply SEO metadata; `canonicalPath` preserves the public URL. Guide pages also have `heading` and `intro` fields.

Eleventy uses Liquid only for shared layouts and the sitemap. If a code example needs literal Liquid syntax such as `{{ ... }}` or `{% ... %}`, surround that example with `{% raw %}` and `{% endraw %}`.

To add a guide, copy an existing HTML file in `pages/docs/`, update its metadata and content, and add its link in `pages/_includes/guide.html`. The sitemap includes it automatically. A file such as `pages/docs/docker.html` generates `_site/docs/docker/index.html`; the existing `/docs/docker` links work on Cloudflare Pages and in the local preview.

`assets/social-preview.html` is the source artwork for the social image. It is not published by the site build.

## Build and deployment

```sh
yarn build
```

The complete website is written to `_site/`. Edit the source files above; changes made inside `_site/` are overwritten. `yarn generate` is an alias for the same build.

```sh
yarn deploy
```

This builds the site and uploads `_site/` to the existing Cloudflare Pages project using Wrangler. It requires Cloudflare authentication and publishes to the `master` branch. Wrangler remains a deployment tool invoked through `npx`; Eleventy is the only direct project dependency.

The tag-triggered release workflow also runs `yarn deploy` from `site/` after creating the GitHub release. Configure these repository secrets in GitHub Actions:

- `CLOUDFLARE_ACCOUNT_ID`: the account containing the `endpoll` Pages project.
- `CLOUDFLARE_API_TOKEN`: an API token with **Account → Cloudflare Pages → Edit** permission for that account. See [Cloudflare's CI deployment guide](https://developers.cloudflare.com/pages/how-to/use-direct-upload-with-continuous-integration/).

Deployment paths and Cloudflare options stay in the `deploy` script in `package.json`.
