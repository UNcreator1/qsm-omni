# Data Lake Materials Requirement

Date: 2026-05-03

Phase A is not just brainstorming. It is procurement-grade planning.

Before QSM starts `Chop`, the Data Lake must prove that the project has the right materials and that those materials are current enough to build with.

## Material Definition

A material is anything required to build, test, audit, or deliver the product:

- framework
- package/library
- runtime
- CLI tool
- browser driver
- model route/API
- security scanner
- benchmark/eval dataset
- repo module or internal API
- documentation/source reference

## Required Checks

Every critical material must have:

- `name`
- `purpose`
- `criticality`: `critical`, `optional`, or `enterprise-only`
- `selected_version` or `local_version`
- `verification_source`
- `checked_at`
- `freshness_status`: `fresh`, `stale`, `unverified`, or `blocked`
- `availability_status`: `available`, `missing`, `rate-limited`, `unauthorized`, or `unknown`
- `compatibility_notes`
- `security_notes`
- `alternatives_considered`
- `selection_reason`
- `blocker_reason` when not usable

## Freshness Rules

Use primary sources when available:

- official docs for framework/API behavior;
- package registry or local package manager for versions;
- `qsm route-health` for model/API routes;
- local CLI version commands for installed tools;
- security scanner output for dependency risks.

Default freshness policy:

- official docs or API specs: re-check when older than 30 days;
- package/tool versions: re-check when older than 14 days;
- model/API route health: re-check before every real harness run;
- browser/security tools: re-check before production-profile run;
- benchmark datasets: pin version/hash and re-check before publishing claims.

## Build Gate

No `Chop` when a critical material is:

- `blocked`;
- `unverified`;
- unavailable;
- incompatible with the selected harness/profile;
- known insecure with high/critical findings.

Allowed:

- optional materials may be `GAP`;
- enterprise-only materials may be `LOCAL-GAP` in local-dev profile;
- local-dev may proceed without browser/security tools only if the Force Checklist records the missing evidence.

## Required Lake Artifacts

The planning gate must write:

- `materials_inventory`
- `materials_quality_audit`
- `resource_freshness_report`
- `chop_readiness_verdict`

The `chop_readiness_verdict` must explicitly say:

- which materials are approved;
- which are missing;
- which are stale;
- whether alternatives were considered;
- whether build can start.
