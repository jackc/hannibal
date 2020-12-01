create function http_post_raw_args(
  raw_args jsonb,
  out resp_body jsonb
)
language plpgsql as $$
begin
  select raw_args into resp_body;
end;
$$;
