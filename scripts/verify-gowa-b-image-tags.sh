#!/usr/bin/env bash
# Hermetic contract for the GOWA Blue (B) image publish, cloned from
# vozclara-api/scripts/verify-api-b-image-tags.js:
#   - docker-publish-b.yml exists and fires only on deploy/gowa-b
#   - docker-publish-b.yml never mentions main/master
#   - the required B tags are present (:b and :b-${{ github.sha }})
#   - docker-publish-b.yml never publishes :latest, :sha-, or an unprefixed sha tag
#   - build-push-main.yml never emits :b or :b-<sha>
#   - the immutable deploy pin is b-<fullsha>; the mutable :b tag is never a deploy pin
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
b_workflow="$root/.github/workflows/docker-publish-b.yml"
main_workflow="$root/.github/workflows/build-push-main.yml"

fail() {
  echo "verify-gowa-b-image-tags: $*" >&2
  exit 1
}

[ -f "$b_workflow" ] || fail "missing $b_workflow"
[ -f "$main_workflow" ] || fail "missing $main_workflow"

# The B workflow must fire only on deploy/gowa-b (plus manual dispatch).
grep -Eq 'branches:[[:space:]]*\[deploy/gowa-b\][[:space:]]*$' "$b_workflow" \
  || fail "docker-publish-b.yml must fire only on deploy/gowa-b"
grep -q 'workflow_dispatch:' "$b_workflow" \
  || fail "docker-publish-b.yml must keep workflow_dispatch"

# The B workflow must not mention main/master (no A/Main coupling).
if grep -Eq '(^|[^A-Za-z])(main|master)([^A-Za-z]|$)' "$b_workflow"; then
  fail "docker-publish-b.yml must not mention main/master"
fi

# Required tags.
for tag in \
  'ghcr.io/djeyff/go-whatsapp-multidevice:b' \
  'ghcr.io/djeyff/go-whatsapp-multidevice:b-${{ github.sha }}'; do
  grep -Fq "$tag" "$b_workflow" \
    || fail "docker-publish-b.yml missing required tag $tag"
done

# The B workflow may only publish :b and :b-<...> tags — never :latest,
# :sha-<...>, or an unprefixed sha tag.
bad_tags="$(
  grep -Eo 'go-whatsapp-multidevice:[^[:space:]]+' "$b_workflow" \
    | grep -Ev 'go-whatsapp-multidevice:b(-|$)' || true
)"
if [ -n "$bad_tags" ]; then
  fail "docker-publish-b.yml must publish only :b and :b-<sha> tags (found: $bad_tags)"
fi

# The main workflow must never emit b or b-<sha> tags.
if grep -Fq 'go-whatsapp-multidevice:b' "$main_workflow" \
  || grep -Fq 'b-${{ github.sha }}' "$main_workflow"; then
  fail "build-push-main.yml must never emit b or b-<sha> tags"
fi

# RET-221 pin contract: the immutable tag is the full-SHA tag
# b-${{ github.sha }}; the mutable :b tag must never be described as a deploy pin.
grep -Fq 'ghcr.io/djeyff/go-whatsapp-multidevice:b-${{ github.sha }}' "$b_workflow" \
  || fail "docker-publish-b.yml must tag ghcr.io/djeyff/go-whatsapp-multidevice:b-<fullsha> (github.sha) for immutable digest deploys"
mutable_pins="$(
  grep -E 'image:' "$b_workflow" \
    | grep -F 'go-whatsapp-multidevice:b' \
    | grep -vF 'go-whatsapp-multidevice:b-' || true
)"
if [ -n "$mutable_pins" ]; then
  fail "deploy pins must use b-<fullsha>; the mutable b tag must never be a deploy pin"
fi

echo "verify-gowa-b-image-tags: ok"
