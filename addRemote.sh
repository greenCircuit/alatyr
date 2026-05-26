#!/bin/bash
set -e
projectName=${1:?provide project name}
echo "projectName: ${projectName}"

gh="https://github.com/greenCircuit/${projectName}"
gl="https://gitlab.dev.io/home-lab/${projectName}"

git remote remove origin 2>/dev/null || true
git remote remove gitlab 2>/dev/null || true
git remote remove all    2>/dev/null || true

git remote add origin "$gh"
git remote add gitlab "$gl"

# 'all' remote: fetch from github, push to both
git remote add all "$gh"
git remote set-url --add --push all "$gh"
git remote set-url --add --push all "$gl"

git push --set-upstream all main
git remote -v
