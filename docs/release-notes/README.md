# Release notes

This folder holds the text shown in the app under **What's new** and used as the
body of the GitHub release.

- One file per version: `<version>.md` (for example `1.4.1.md`). The file name
  has no `v`; the tag is `v<version>`.
- English, Markdown.
- Written before the release. When `VERSION` is changed on `main`, the release
  workflow reads the file of that version and publishes it.
- If the file is missing, the workflow does not fail: it writes a
  "What's new in <version>" text with the commit subjects since the previous tag.
  Add the file to get a proper text.

## If a release run does not happen

The release workflow runs one release at a time and never cancels a run in
progress. GitHub keeps only one pending run per queue, though: if two version
pushes arrive while a release is still running, the older pending run can be
dropped. Its version then is not published.

To publish it again, when its tag does not exist yet:

1. Make sure `VERSION` on `main` has that version.
2. In Actions, open **Release**, choose **Run workflow**, branch `main`, and
   uncheck `dry_run`.

A run that finds the version already published does nothing (or fails, when it
was started by hand with `dry_run` unchecked). A leftover draft or a tag without
a published release does not block a new run.
