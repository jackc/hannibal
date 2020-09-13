create function http_api_register_user(
  args jsonb,
  out cookie_session jsonb,
  out resp_body jsonb
)
language plpgsql as $$
declare
  _user_id int;
begin
  insert into users (username, password_digest)
  values (args ->> 'username', args ->> 'passwordDigest')
  returning id
  into strict _user_id;

  cookie_session := jsonb_build_object('user_id', _user_id);
  resp_body := jsonb_build_object('user_id', _user_id);
end;
$$;

create function get_user_password_digest(
  args jsonb
) returns text
language plpgsql as $$
declare
  _password_digest text;
begin
  select password_digest
  into _password_digest
  from users
  where username = args ->> 'username';

  if not found then
    return 'not found';
  end if;

  return _password_digest;
end;
$$;

create function http_api_login(
  args jsonb,
  inout cookie_session jsonb,
  out status smallint
)
language plpgsql as $$
declare
  _user_id int;
begin
  if not (args ->> 'validPassword')::boolean then
    status := 400;
    return;
  end if;

  select id
  into strict _user_id
  from users
  where username = args ->> 'username';

  cookie_session := jsonb_build_object('user_id', _user_id);

  status := 200;
end;
$$;
