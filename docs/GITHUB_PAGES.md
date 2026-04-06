# Publishing docs on GitHub Pages

This repository’s documentation lives under **`docs/`**. You can serve it as a site at:

`https://<your-username>.github.io/<repository-name>/`

## Steps

1. Push these files to GitHub (`docs/_config.yml`, `docs/index.md`, etc.).
2. On GitHub: **Settings** → **Pages** (left sidebar).
3. Under **Build and deployment** → **Source**, choose **GitHub Actions** *or* **Deploy from a branch**:
   - **Branch:** `main` (or your default branch).
   - **Folder:** `/docs` (the **`/docs`** folder in the root of the repo).
4. Save. After a minute or two, GitHub shows the site URL at the top of the Pages settings page.

## If the site looks broken (CSS or links)

In **`docs/_config.yml`**, set **`baseurl`** to your repository name with slashes:

```yaml
baseurl: "/YOUR-REPO-NAME"
```

Example: repo `https://github.com/jane/orderbook` → `baseurl: "/orderbook"`.

If the repo is **`username.github.io`**, use `baseurl: ""` (user site, not a project site).

## Mermaid diagrams

GitHub’s **file** view renders Mermaid in Markdown. The default Jekyll theme here may **not** render Mermaid on the Pages site—open the doc on GitHub or rely on the architecture section in ORDERBOOK as raw Markdown.

## Alternative: docs URL without Pages

You can always link the **About** “Website” field or your README to:

`https://github.com/<user>/<repo>/tree/main/docs`

No build step required.
