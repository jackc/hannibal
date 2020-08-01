select hannibal_system.create_handler(
  'GET',
  '/hey',
  'select json_build_object(''time'', now(), ''name'', $1::text, ''foo'', ''def'')',
  array[
    hannibal_system.build_handler_param('name', 'text', true)
  ]
);

create function sub(int, int) returns int
language sql as $$
  select $1 - $2;
$$;
