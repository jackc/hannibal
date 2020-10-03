create function http_api_create_todo(
  args jsonb,
  arg_errors jsonb,
  out resp_body jsonb
)
language plpgsql as $$
begin
  if arg_errors is not null then
    resp_body = jsonb_build_object('error', arg_errors);
    return;
  end if;

  insert into todos (name)
  values (args ->> 'name')
  returning jsonb_build_object(
    'id', id,
    'name', name
  ) into resp_body;
end;
$$;
