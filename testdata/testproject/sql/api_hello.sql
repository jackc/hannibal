create function api_hello(
  args jsonb,
  out resp_body jsonb
)
language plpgsql as $$
begin
  select
    jsonb_build_object(
      'name', args ->> 'name'
    )
  into resp_body;
end;
$$;
