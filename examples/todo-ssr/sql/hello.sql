create function hello(
  query_args jsonb,
  out template text,
  out template_data jsonb
)
language plpgsql as $$
begin
  select
    'hello.html',
    jsonb_build_object('time', now(), 'name', query_args ->> 'name')
  into template, template_data;
end;
$$;
