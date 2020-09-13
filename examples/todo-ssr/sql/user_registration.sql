create function http_user_registration(
  args jsonb,
  inout cookie_session jsonb,
  out template text,
  out template_data jsonb
)
language plpgsql as $$
begin
  if cookie_session is null then
    cookie_session := jsonb_build_object('visitCount', 0);
  end if;
  cookie_session := jsonb_set(cookie_session, '{visitCount}', to_jsonb((cookie_session ->> 'visitCount')::int + 1));

  select
    'hello.html',
    jsonb_build_object(
      'time', now(),
      'name', args ->> 'passwordDigest',
      'visitCount', cookie_session -> 'visitCount'
    )
  into template, template_data;
end;
$$;
