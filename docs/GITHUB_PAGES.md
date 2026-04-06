# Publishing documentation on GitHub Pages

**This repository’s live documentation:**

**https://mehdishariati.github.io/orderbook/**

Generic pattern: **`https://<username>.github.io/<repository>/`**

---

## Recommended: GitHub Actions (this repo includes a workflow)

1. Push the latest code (including `.github/workflows/pages.yml` and `docs/`).
2. On GitHub: **Settings** → **Pages**.
3. Under **Build and deployment** → **Source**, select **GitHub Actions** (not “Deploy from a branch” if you want this workflow to own deploys).
4. Open the **Actions** tab → run **Deploy documentation to Pages** (or push to `main` / `master` to trigger it).
5. When the job is green, **Pages** settings shows the live URL. Paste that URL into the repo **About** → **Website** field.

**If the site has no styling** (plain HTML): edit **`docs/_config.yml`** and set **`baseurl`** to `"/YOUR-REPO-NAME"` (leading slash, exact GitHub repo name). Commit and push; the workflow rebuilds.

---

## Alternative: deploy from the `docs/` folder (no Actions)

1. **Settings** → **Pages** → **Source:** **Deploy from a branch**.
2. Branch: **`main`** or **`master`**, folder **`/docs`**.
3. Save and wait for the build.

Use **either** Actions **or** branch deploy—not both as competing sources.

---

## Local preview (optional)

```bash
cd docs
bundle install
bundle exec jekyll serve
```

Open `http://127.0.0.1:4000/orderbook/` (path includes **`baseurl`**).

---

## Mermaid diagrams

**ORDERBOOK.md** uses Mermaid. GitHub’s **file view** renders Mermaid; the Jekyll site may show diagrams as **fenced code** unless you add a Mermaid plugin. For diagrams, prefer opening the **`.md` file on GitHub** or read the source.

---

## Copy-paste for the About box

**Website:** `https://<username>.github.io/<repository>/`  
More options: [GITHUB_ABOUT.md](GITHUB_ABOUT.md)
