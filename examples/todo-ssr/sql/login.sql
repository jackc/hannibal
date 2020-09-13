create function get_user_password_digest(
  args jsonb
) returns text
language plpgsql as $$
declare
  result jsonb;
begin
  select 'not found'
  into result;

  return result;
end;
$$;
