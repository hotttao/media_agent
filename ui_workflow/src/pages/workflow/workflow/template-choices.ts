/** Reading a legacy template is not permission to bind it into a new company workflow. */
export function isCompanyTemplate(organizationId: string, templateOrganizationId?: string) {
  return Boolean(organizationId) && templateOrganizationId === organizationId;
}
