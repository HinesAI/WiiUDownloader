# Upstream review (Xpl0itU)

Once a month we review [Xpl0itU/WiiUDownloader](https://github.com/Xpl0itU/WiiUDownloader) for changes worth bringing into this fork.

## Automation

GitHub Actions workflow **Upstream check** runs on the **1st of each month** (and via `workflow_dispatch`). It:

1. Fetches upstream `main`
2. Compares against our `HEAD` (merge-base / commit list / diffstat)
3. Opens or updates a GitHub issue labeled `upstream-sync`

Locally:

```bash
./scripts/check-upstream.sh
```

Optional remote (already added by the script if missing):

```bash
git remote add upstream https://github.com/Xpl0itU/WiiUDownloader.git
```

## Decision rubric

| Choice | When |
|--------|------|
| **Merge** | Small, compatible changes; low conflict with our UI/branding/CI |
| **Selective port** | Only some commits matter; cherry-pick or re-implement |
| **Skip** | Cosmetic upstream churn, or nothing relevant to our fork |

Prefer selective port when upstream touches areas we’ve heavily forked (GTK UI, GameTDB, packaging, branding). Prefer merge for shared core download/decrypt fixes.

## After deciding

Comment on the `upstream-sync` issue with the choice, link any PR, then close the issue when that month’s review is done.
