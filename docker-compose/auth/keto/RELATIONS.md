# Keto relation model

Keto v26 stores relation tuples without loading legacy OPL namespace files. The
`media_auth` projection writes the following canonical namespaces and relations.

| Namespace | Relations | Meaning |
| --- | --- | --- |
| `Organization` | `member`, `admin`, `materials_entitled`, `workflow_entitled` | Company membership, administrators, and capability ceiling |
| `Team` | `organization`, `member`, `admin`, `materials_user`, `workflow_user`, `workspace_creator` | Department ownership and effective capability projection |
| `Workspace` | `team`, `viewer`, `editor`, `admin` | Workspace department ownership and access projection |

`Organization` is a company and `Team` is a department. A Team belongs to one
Organization; a Workspace belongs to one Team. Team effective capability tuples
are projections of the Organization ceiling and Team inherit/restricted mode.
They are not an independent source of truth. Resource-specific grants remain in
the service that owns the resource.
