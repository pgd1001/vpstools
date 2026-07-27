#!/bin/sh
set -eu

evidence_file=${1:-docs/release-evidence-template.md}
template_mode=${2:-}
[ -f "$evidence_file" ] || { echo "release evidence file not found: $evidence_file" >&2; exit 1; }

required_fields='Version, exact release tag:
Git commit, full SHA:
CI run URL:
Checksums file, path or URL:
SBOM document(s), path or URL:
Target-host acceptance output:
Backup and restore result:
Measured RPO:
Measured RTO:
Rollback result:
Identity-provider verification result:
Accepted residual risks, or `None`:'

failures=0
values_file=$(mktemp "${TMPDIR:-/tmp}/vps-tools-release-values.XXXXXX")
trap 'rm -f "$values_file"' EXIT HUP INT TERM
while IFS= read -r field; do
    [ -n "$field" ] || continue
    line_present=$(grep -F -c -- "- $field" "$evidence_file" || true)
    if [ "$line_present" -eq 0 ]; then
        echo "missing release evidence field label: $field" >&2
        failures=$((failures + 1))
        continue
    fi
    value=$(awk -v prefix="- $field" 'index($0, prefix) == 1 { sub("^[^:]*: *", ""); print; exit }' "$evidence_file")
    if [ -z "$value" ]; then
        if [ "$template_mode" = --template ]; then continue; fi
        echo "missing release evidence field: $field" >&2
        failures=$((failures + 1))
    elif [ "$template_mode" != --template ] && printf '%s\n' "$value" | grep -Eiq '^(TBD|N/A|not run|todo|<.*>)$'; then
        echo "placeholder release evidence field: $field" >&2
        failures=$((failures + 1))
    fi
    printf '%s\t%s\n' "$field" "$value" >> "$values_file"
done <<EOF
$required_fields
EOF

value_for() {
    awk -F '\t' -v wanted="$1" '$1 == wanted { print substr($0, index($0, "\t") + 1); exit }' "$values_file"
}

if [ "$template_mode" != --template ]; then
    commit=$(value_for 'Git commit, full SHA:')
    printf '%s\n' "$commit" | grep -Eiq '[0-9a-f]{40}' || { echo "Git commit must include a 40-character commit hash" >&2; failures=$((failures + 1)); }
    ci_url=$(value_for 'CI run URL:')
    printf '%s\n' "$ci_url" | grep -Eiq 'https://[^[:space:]]+/actions/runs/[0-9]+' || { echo "CI run URL must point to a GitHub Actions run" >&2; failures=$((failures + 1)); }

    for field in 'Checksums file, path or URL:' 'SBOM document(s), path or URL:' 'Target-host acceptance output:' 'Backup and restore result:' 'Rollback result:' 'Identity-provider verification result:'; do
        value=$(value_for "$field")
        printf '%s\n' "$value" | grep -Eiq '(^|[^[:alnum:]])PASS([^[:alnum:]]|$)' || { echo "gate is not recorded as PASS: $field" >&2; failures=$((failures + 1)); }
        printf '%s\n' "$value" | grep -Eiq '(^|[[:space:],])(https?://|[[:alnum:]_.-]+/|[[:alnum:]_.-]+\.(txt|json|md|log))' || { echo "gate has no retained evidence path or URL: $field" >&2; failures=$((failures + 1)); }
    done

    for field in 'Measured RPO:' 'Measured RTO:'; do
        value=$(value_for "$field")
        printf '%s\n' "$value" | grep -Eiq '[0-9]+[[:space:]]*(second|minute|hour|day|week)s?' || { echo "measured duration is missing or invalid: $field" >&2; failures=$((failures + 1)); }
    done
fi

if [ "$template_mode" != --template ]; then
    commit=$(awk 'index($0, "- Git commit, full SHA:") == 1 { sub("^[^:]*: *", ""); print; exit }' "$evidence_file")
    printf '%s\n' "$commit" | grep -Eiq '^[0-9a-f]{40}$' || { echo "Git commit must be a full 40-character SHA" >&2; failures=$((failures + 1)); }
    ci_url=$(awk 'index($0, "- CI run URL:") == 1 { sub("^[^:]*: *", ""); print; exit }' "$evidence_file")
    printf '%s\n' "$ci_url" | grep -Eiq '^https?://' || { echo "CI run URL must be an HTTP(S) URL" >&2; failures=$((failures + 1)); }
fi

awk -F'|' '
    /\|[[:space:]]*Gate[[:space:]]*\|/ { in_table=1; next }
    in_table && /^\|/ {
        gate=$2; status=$3; evidence=$4
        if (status ~ /\[[xX]\]/ && evidence !~ /[^[:space:]-]/) {
            printf "checked environment-only gate has no evidence: %s\n", gate > "/dev/stderr"
            bad=1
        }
    }
    END { exit bad }
' "$evidence_file" || failures=$((failures + 1))

[ "$failures" -eq 0 ] || exit 1
echo "release evidence validated: $evidence_file"
