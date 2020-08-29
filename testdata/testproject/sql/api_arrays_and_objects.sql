create function api_arrays_and_objects(
  args jsonb,
  out resp_body jsonb
)
language plpgsql as $$
begin
  select args into resp_body;
end;
$$;
