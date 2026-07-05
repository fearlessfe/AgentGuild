#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."

cd backend && go test -race ./... -count=1
cd ../frontend && npm run test -- --run
cd ../frontend && npx playwright test e2e/agent-version-and-experience.spec.ts
