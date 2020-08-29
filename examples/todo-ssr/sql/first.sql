create function get_time(
  args jsonb,
  out resp_body jsonb
)
language plpgsql as $$
begin
  select jsonb_build_object('time', now(), 'name', args ->> 'name')
  into resp_body;
end;
$$;
