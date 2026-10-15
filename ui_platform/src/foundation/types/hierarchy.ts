export interface Organization {
  id: string;
  name: string;
  status: string;
  my_role: string;
}
export interface Team {
  id: string;
  name: string;
  organization_id: string;
  status: string;
  my_role: string;
}
