import { Namespace, Context, SubjectSet } from "@ory/keto-namespace-types"

class User implements Namespace {}

class Organization implements Namespace {
  related: {
    members: User[]
    admins: User[]
    materials_entitled: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]
    workflow_entitled: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]
  }

  permits = {
    view: (ctx: Context): boolean =>
      this.related.members.includes(ctx.subject) || this.related.admins.includes(ctx.subject),
    manage: (ctx: Context): boolean => this.related.admins.includes(ctx.subject),
  }
}

class Team implements Namespace {
  related: {
    organization: Organization[]
    members: User[]
    admins: User[]
    materials_users: (SubjectSet<Team, "members"> | SubjectSet<Team, "admins">)[]
    workflow_users: (SubjectSet<Team, "members"> | SubjectSet<Team, "admins">)[]
    workspace_creators: (SubjectSet<Team, "members"> | SubjectSet<Team, "admins">)[]
  }

  permits = {
    view: (ctx: Context): boolean =>
      this.related.members.includes(ctx.subject) || this.related.admins.includes(ctx.subject),
    manage: (ctx: Context): boolean => this.related.admins.includes(ctx.subject),
    use_materials: (ctx: Context): boolean => this.related.materials_users.includes(ctx.subject),
    use_workflow: (ctx: Context): boolean => this.related.workflow_users.includes(ctx.subject),
    create_workspace: (ctx: Context): boolean => this.related.workspace_creators.includes(ctx.subject),
  }
}

class Workspace implements Namespace {
  related: {
    team: Team[]
    viewers: User[]
    editors: User[]
    admins: User[]
  }

  permits = {
    view: (ctx: Context): boolean =>
      this.related.viewers.includes(ctx.subject) ||
      this.related.editors.includes(ctx.subject) ||
      this.related.admins.includes(ctx.subject),
    edit: (ctx: Context): boolean =>
      this.related.editors.includes(ctx.subject) || this.related.admins.includes(ctx.subject),
    manage: (ctx: Context): boolean => this.related.admins.includes(ctx.subject),
  }
}
