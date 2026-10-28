# App types: what goes live on its own and what needs approval

Security approves what a kind of app may do, not each app's code. It does that with app types:
an app type says who its apps may serve, the data they may hold, where their mail may go and
which outside services they may call. When a publish passes the security checks, the platform
compares the app's permission list with its app type and decides whether the release is
*Pre-approved*, *Needs approval* or *Not allowed*. This guide covers app types, that decision,
the group map, the source registry, and what `aicoded publish` shows.

Your organisation's security admin defines its app types, its group map and its source
registry. Until the console exists, the platform's operator sets them for your organisation, and
every change has a change record.

## App types

`class` in `aicoded.yaml` names the app's type. An app without `class` is an *Internal tool*,
`internal-tool`, which every organisation has:

| The app type allows | Internal tool |
|---|---|
| Who uses it, `audience.internal` | anyone in the company: `everyone` and every group |
| Data, the highest class in `data` | `internal` |
| Mail, `email.to_domains` | the company's own domains: those its people sign in with, and its verified mail domains |
| Outside services, `egress` | none |

Your security admin may define other types, such as `finance`, with other limits, and may change
the Internal tool for your organisation. A `class` that your organisation has no type of fails the
publish (E-REL-019), and the problem lists the types it has. An AI assistant leaves `class` out
unless you or your administrator name a type.

## The decision

When a publish passes, `aicoded publish` and `aicoded status` show one of these before the status
line:

- **Pre-approved.** The permission list stays within the app type, and nothing else needs an
  approver. The platform releases it on its own, with no one to ask:

  ```text
    Pre-approved by app type internal-tool: going live.
  ```

  The line then says `live` once the new version serves.

- **Needs approval.** The permission list asks for more than the app type allows, or the release
  holds a change that always needs an approver. Each reason is a line with its code:

  ```text
    Needs approval:
      E-GATE-041: mail to partner.example, which the app type internal-tool does not allow
    An approver must approve it before it goes live.
  ```

  An approver sees an approval card that lists only what is new compared with the app type.
  Until the console exists, the platform's operator releases such a publish when your
  organisation asks. The reasons are notes of the step `decision` too, each with its fix.

- **Not allowed.** Some things no app may do, whatever its type: an audience that lists no one,
  and health, HR or payment card data. `aicoded check` refuses them in the permission list
  (E-MAN-024), and the source registry can mark a source so (E-REL-021).

A publish that passes but is not released says `Not released:` and why, such as a newer publish
of the app that came after it, or what the platform does not run yet, such as outside services
(E-REL-017).

### What always needs approval

Whatever the app type allows, a release needs approval when:

- people outside the company may use the app (E-GATE-043). For now no release with an
  `audience.external` goes live (E-REL-004);
- its schema changes hold a destructive one (E-GATE-044, E-MIG-003);
- data from the app's database, files, inbox or secrets reaches mail in a way the live version
  did not have (E-GATE-045, E-DATA-001);
- a page is open to more people than in the live version (E-GATE-046): its roles are not among
  those it had, it loses its `Guard`, or it becomes `shared`. A new page is not.

### What the app type decides

Each of these is *Pre-approved* within the app type, and needs approval beyond it:

- an entry of `audience.internal` the type does not allow (E-GATE-039);
- a data class above the type's highest one, in `data` or in the source registry (E-GATE-040);
- a domain in `email.to_domains` the type does not allow (E-GATE-041);
- a host in `egress` the type does not allow (E-GATE-042).

To stay *Pre-approved*, keep the permission list within the app type, or ask your security admin
for an app type that fits the app.

### When things change before the release

The platform decides again when it releases, since the app type, the group map or the live version
may have changed since the publish passed. A release that was *Pre-approved* and now needs
approval waits, and `aicoded status` shows the new reasons. A newer publish of the app replaces
an older one that has not gone live yet.

A rollback releases an earlier publish again, decided against the app type as it is now. One past
a destructive schema change that ran is refused (E-MIG-007).

## The group map

People sign in with the company login, whose groups the app never sees. The group map gives
names to them: each name stands for a group of the company login or, in an organisation of
invited people, for a list of them. The app uses only these names:

- `group:<name>` in `audience.internal` is the people of the group map's `<name>`;
- a role in an access rule, `role="hr"`, is membership of the group map's `hr`;
- `everyone` is every person of the organisation, and needs no name.

A group or a role that the group map does not have fails the publish (E-REL-020), and the problem
lists the names it has. On the platform, `auth.Viewer(ctx)` gives the names the viewer is in, of
those the app uses, as both `Roles` and `Groups`. They are checked again every 15 minutes: a
viewer who left a group loses its role, and one who left the app's audience is signed out of it
and sees E-RUN-006.

## The source registry

The classes in `data` are what the builder declares. Security knows better what some sources
hold, so the source registry lets your security admin set the class of each of an app's sources,
such as `sqldb` or `filestore:invoices`. The highest of the declared class and the registry's
class counts:

- a registry class above the app type's highest one needs approval (E-GATE-040), even when
  `data` says less;
- `health`, `hr` or `card` in the registry is *Not allowed* (E-REL-021): the app is never
  released.

Declare the classes your data really has, so the decision matches what security expects.

## The codes

- E-REL-019, E-REL-020 and E-REL-021: a publish that fails when it is decided, and a release
  that is refused for the same reason.
- E-GATE-039 to E-GATE-046: the reasons a release needs approval, as notes, which do not stop a
  publish.
- E-MAN-024: what is *Not allowed* in any permission list.
- E-RUN-006: the page of a viewer outside the app's audience.

## See also

- [The permission list](permission-list.md): every section of `aicoded.yaml`.
- [Publishing](publishing.md): what a publish checks and how to read its outcome.
- [Access](access.md): access rules, roles and Guards.
- [Schema changes](schema-changes.md): destructive changes and why they need approval.
