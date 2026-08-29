#!/usr/bin/env bash
#
# Prune old images from the project's Vercel Container Registry repository.
#
# Every deployment pushes a new image tagged with the commit sha, and nothing
# removes the old ones. The repository has a hard cap on image count, and once
# it is reached every build fails at the push step with
# `denied: repository has reached the maximum allowed number of images`.
#
# Keeps the KEEP newest images by creation time and deletes the rest. The rule
# is count-based on purpose: the cap is a count, so an age-based rule ("older
# than N days") can still overflow during a burst of deployments.
#
# Usage:
#   DRY_RUN=1 scripts/prune-vercel-images.sh   # report only, delete nothing
#   KEEP=10 scripts/prune-vercel-images.sh
#
# Requires the `vercel` CLI. In CI, set VERCEL_TOKEN; locally, `vercel login`
# is enough.

set -euo pipefail

REPOSITORY="${VCR_REPOSITORY:-dockerfile}"
PROJECT="${VERCEL_PROJECT:-d3d-api}"
SCOPE="${VERCEL_SCOPE:-smetroids-projects}"
KEEP="${KEEP:-25}"
DRY_RUN="${DRY_RUN:-0}"

# The API caps a single page at 100. The registry cap is lower than that today,
# so one page is the whole repository; warn rather than silently prune a
# partial view if that ever stops being true.
PAGE_LIMIT=100

if ! [[ "$KEEP" =~ ^[0-9]+$ ]] || [ "$KEEP" -lt 1 ]; then
    echo "KEEP must be a positive integer, got '$KEEP'" >&2
    exit 1
fi

if ! command -v vercel >/dev/null 2>&1; then
    echo "vercel CLI not found on PATH" >&2
    exit 1
fi

vercel_args=(--project "$PROJECT" --scope "$SCOPE")
if [ -n "${VERCEL_TOKEN:-}" ]; then
    vercel_args+=(--token "$VERCEL_TOKEN")
fi

echo "Repository : $REPOSITORY (project $PROJECT, scope $SCOPE)"
echo "Keeping    : $KEEP newest images"
[ "$DRY_RUN" != "0" ] && echo "Mode       : DRY RUN, nothing will be deleted"

if ! images_json=$(vercel vcr image ls "$REPOSITORY" "${vercel_args[@]}" \
        --json --limit "$PAGE_LIMIT" 2>/dev/null); then
    echo "Failed to list images in '$REPOSITORY'" >&2
    exit 1
fi

# Sorting happens here rather than trusting the API's order: deleting the wrong
# end of the list would destroy the newest images, including whatever is
# serving production.
if ! doomed=$(printf '%s' "$images_json" | KEEP="$KEEP" PAGE_LIMIT="$PAGE_LIMIT" python3 -c '
import json, os, sys

keep = int(os.environ["KEEP"])
page_limit = int(os.environ["PAGE_LIMIT"])

try:
    images = json.load(sys.stdin)["images"]
except (ValueError, KeyError, TypeError) as exc:
    sys.exit("could not parse image list: %s" % exc)

if not isinstance(images, list):
    sys.exit("expected a list of images")

if len(images) >= page_limit:
    print("WARNING: hit the %d-image page limit; older images may not be "
          "listed and will not be pruned" % page_limit, file=sys.stderr)

for image in images:
    if not image.get("id") or not image.get("createdAt"):
        sys.exit("image record missing id or createdAt: %r" % image)

images.sort(key=lambda i: i["createdAt"], reverse=True)

print(len(images), file=sys.stderr)
for image in images[keep:]:
    print("%s\t%s\t%s" % (image["id"], image["createdAt"],
                          ",".join(image.get("tags") or ["<untagged>"])))
' 2>/tmp/prune-vcr-meta.$$); then
    cat /tmp/prune-vcr-meta.$$ >&2
    rm -f /tmp/prune-vcr-meta.$$
    exit 1
fi

# stderr from the parser carries the total plus any warnings.
total=$(tail -n 1 /tmp/prune-vcr-meta.$$)
sed '$d' /tmp/prune-vcr-meta.$$ >&2
rm -f /tmp/prune-vcr-meta.$$

echo "Found      : $total images"

if [ "$total" -eq 0 ]; then
    echo "Repository is empty; nothing to prune."
    exit 0
fi

if [ -z "$doomed" ]; then
    echo "Nothing to delete: $total image(s) is within the $KEEP kept."
    exit 0
fi

count=$(printf '%s\n' "$doomed" | grep -c .)
echo "Deleting   : $count image(s)"
echo

deleted=0
failed=0
while IFS=$'\t' read -r id created tags; do
    [ -z "$id" ] && continue
    if [ "$DRY_RUN" != "0" ]; then
        echo "  would delete $id  $created  $tags"
        continue
    fi
    if vercel vcr image rm "$REPOSITORY" "$id" "${vercel_args[@]}" --yes >/dev/null 2>&1; then
        echo "  deleted $id  $created  $tags"
        deleted=$((deleted + 1))
    else
        echo "  FAILED  $id  $created  $tags" >&2
        failed=$((failed + 1))
    fi
done <<< "$doomed"

echo
if [ "$DRY_RUN" != "0" ]; then
    echo "Dry run complete: $count image(s) would be deleted, $((total - count)) kept."
    exit 0
fi

echo "Deleted $deleted image(s), $failed failure(s), $((total - deleted)) remaining."
[ "$failed" -gt 0 ] && exit 1
exit 0
