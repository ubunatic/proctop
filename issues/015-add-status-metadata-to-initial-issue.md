# 015 — Add status metadata to initial issue

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Hygiene
**Related**:

---

## 1. Problem & Motivation

`issues/README.md` says each issue has a `Status:` line, but the genesis
prompt in `issues/000-init.md` has no status. Its lifecycle state is unclear,
and the tracker convention is inconsistent.

## 2. Technical Specification / Findings

This is a documentation hygiene gap. The genesis prompt describes the MVP,
which is implemented, so `done` is likely the correct status; alternatively,
document an explicit exception for genesis records.

## 3. Implementation & Verification Plan

Add the chosen status to issue 000 or document the exception. Keep the issue
index and linter expectations consistent.
