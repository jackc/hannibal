select foobarbuilder.create_handler(
  'GET',
  '/hey',
  'select json_build_object(''time'', now(), ''name'', $1::text)',
  array[
    foobarbuilder.build_handler_param('name', '[]'::jsonb)
  ]
);
