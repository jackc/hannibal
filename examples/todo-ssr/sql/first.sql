create function get_time(
  query_args jsonb,
  out resp_body jsonb
)
language plpgsql as $$
begin
  select jsonb_build_object('time', now(), 'name', query_args ->> 'name')
  into resp_body;
end;
$$;
