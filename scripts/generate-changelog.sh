#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly REPO_URL="https://github.com/LiteyukiStudio/luna-devops"
readonly ZH_FILE="${ROOT_DIR}/docs/docs/zh/changelog.md"
readonly EN_FILE="${ROOT_DIR}/docs/docs/en/changelog.md"

section_title_zh() {
  case "$1" in
    added) printf '新增' ;;
    fixed) printf '修复' ;;
    performance) printf '性能' ;;
    docs) printf '文档' ;;
    changed) printf '调整' ;;
    other) printf '其他' ;;
  esac
}

section_title_en() {
  case "$1" in
    added) printf 'Added' ;;
    fixed) printf 'Fixed' ;;
    performance) printf 'Performance' ;;
    docs) printf 'Docs' ;;
    changed) printf 'Changed' ;;
    other) printf 'Other' ;;
  esac
}

category_for_subject() {
  local subject="$1"
  local type=""
  local conventional_re='^([A-Za-z]+)(\([^)]*\))?(!)?([[:space:]][^:]*)?:'

  if [[ "${subject}" =~ ${conventional_re} ]]; then
    type="$(printf '%s' "${BASH_REMATCH[1]}" | tr '[:upper:]' '[:lower:]')"
  fi

  case "${type}" in
    feat) printf 'added' ;;
    fix|revert) printf 'fixed' ;;
    perf) printf 'performance' ;;
    docs) printf 'docs' ;;
    refactor|style|test|build|ci|chore) printf 'changed' ;;
    *) printf 'other' ;;
  esac
}

escape_markdown() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//[/\\[}"
  value="${value//]/\\]}"
  printf '%s' "${value}"
}

write_release() {
  local lang="$1"
  local tag="$2"
  local previous_tag="$3"
  local output_file="$4"
  local range
  local date
  local category
  local hash
  local short_hash
  local subject
  local line
  local commit_url
  local categories=(added fixed performance docs changed other)
  local lines=()

  if [[ -n "${previous_tag}" ]]; then
    range="${previous_tag}..${tag}"
  else
    range="${tag}"
  fi

  date="$(git -C "${ROOT_DIR}" log -1 --format=%cs "${tag}")"

  {
    printf '## %s\n\n' "${tag}"
    if [[ "${lang}" == "zh" ]]; then
      printf '发布日期：%s\n\n' "${date}"
      printf '[查看版本代码](%s/tree/%s)\n\n' "${REPO_URL}" "${tag}"
    else
      printf 'Release date: %s\n\n' "${date}"
      printf '[View tag source](%s/tree/%s)\n\n' "${REPO_URL}" "${tag}"
    fi
  } >> "${output_file}"

  for category in "${categories[@]}"; do
    lines=()
    while IFS= read -r line; do
      lines+=("${line}")
    done < <(
      git -C "${ROOT_DIR}" log --no-merges --format='%H%x1f%s' "${range}" |
        while IFS=$'\x1f' read -r hash subject; do
          [[ "$(category_for_subject "${subject}")" == "${category}" ]] || continue
          printf '%s\t%s\n' "${hash}" "${subject}"
        done
    )

    [[ "${#lines[@]}" -gt 0 ]] || continue

    if [[ "${lang}" == "zh" ]]; then
      printf '### %s\n\n' "$(section_title_zh "${category}")" >> "${output_file}"
    else
      printf '### %s\n\n' "$(section_title_en "${category}")" >> "${output_file}"
    fi

    for line in "${lines[@]}"; do
      hash="${line%%$'\t'*}"
      subject="${line#*$'\t'}"
      short_hash="${hash:0:7}"
      commit_url="${REPO_URL}/commit/${hash}"
      printf -- '- %s ([%s](%s))\n' "$(escape_markdown "${subject}")" "${short_hash}" "${commit_url}" >> "${output_file}"
    done

    printf '\n' >> "${output_file}"
  done
}

generate_file() {
  local lang="$1"
  local output_file="$2"
  local tags=()
  local index
  local tag
  local previous_tag

  mkdir -p "$(dirname "${output_file}")"

  if [[ "${lang}" == "zh" ]]; then
    {
      printf '# 更新日志\n\n'
      printf '这里记录 Luna DevOps 的公开版本变化。最新版本在最上面。\n\n'
    } > "${output_file}"
  else
    {
      printf '# Changelog\n\n'
      printf 'Public release notes for Luna DevOps. The newest release appears first.\n\n'
    } > "${output_file}"
  fi

  while IFS= read -r tag; do
    tags+=("${tag}")
  done < <(git -C "${ROOT_DIR}" tag --list 'v*' --sort=-creatordate)

  if [[ "${#tags[@]}" -eq 0 ]]; then
    if [[ "${lang}" == "zh" ]]; then
      printf '暂时还没有公开版本。\n' >> "${output_file}"
    else
      printf 'No public releases yet.\n' >> "${output_file}"
    fi
    return
  fi

  for index in "${!tags[@]}"; do
    tag="${tags[${index}]}"
    previous_tag=""
    if (( index + 1 < ${#tags[@]} )); then
      previous_tag="${tags[$((index + 1))]}"
    fi
    write_release "${lang}" "${tag}" "${previous_tag}" "${output_file}"
  done
}

cd "${ROOT_DIR}"

generate_file zh "${ZH_FILE}"
generate_file en "${EN_FILE}"
