# Ticket draft: M2 Local Installation Identity

**Source:** Approved M2 Local Installation Identity Design, Target Architecture v0.1.4, and M2 Local Installation Identity Implementation Plan (27 October 2026)
**Review status:** draft

## 1. Persist one installation identity in the Agent's operational state

**What to build:** On first eligible Agent startup, create one durable installation UUID in the existing SQLite database. Repeated and concurrent starts retrieve the same committed identity. Host fingerprint identity remains separate.

**Acceptance criteria**

- [ ] The schema version records the identity table structure, while separate identity state records whether a UUID has been initialized.
- [ ] A fresh installation, an existing M1 database, and a migrated legacy state each receive one valid UUID without changing configuration generations or ownership data.
- [ ] Restarts and simultaneous first-start attempts against one database return the same committed UUID.
- [ ] Missing, malformed, or unreadable initialized identity state produces an explicit error and never silently generates a replacement UUID.
- [ ] UUID creation and validation are independent of SQLite, CLI commands, operating system service code, and the existing host fingerprint.

**Blocked by:** Review of the plan's schema-version versus identity-initialization refinement.

## 2. Require a valid installation identity before Collector runtime starts

**What to build:** Agent startup loads its operational state, ensures the installation identity, and then enters the existing Collector lifecycle. An identity failure stops startup with a clear diagnostic.

**Acceptance criteria**

- [ ] First service startup persists a UUID before Collector supervision begins; missing or damaged operational state does not create an identity.
- [ ] Process restart, service restart, and upgrade or repair with preserved state reuse the same UUID.
- [ ] An identity initialization or validation error prevents Collector launch and is visible in the Agent startup error.
- [ ] Existing CLI behavior, reconciliation generations, Collector configuration, and host telemetry continue to work on Windows and Linux.
- [ ] The documented golden-image flow clones a VM before Agent installation, then installs the Agent separately on each clone so each receives its own UUID.
- [ ] Verification includes focused lifecycle tests, native Windows/Linux checks, a Collector startup smoke check, and the non-test Go file limit of 450 lines.

**Blocked by:** Ticket 1.
