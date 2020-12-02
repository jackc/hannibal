create function http_set_cookie_session(
  raw_args jsonb,
  inout cookie_session jsonb,
  out status smallint
)
language plpgsql as $$
begin
  cookie_session := raw_args;
  status := 200;
end;
$$;

create function http_get_cookie_session(
  inout cookie_session jsonb,
  out resp_body jsonb
)
language plpgsql as $$
begin
  resp_body := cookie_session;
end;
$$;
