drop function if exists create_handler(text, text, text, handler_param[]);
drop function if exists build_handler_param(text, jsonb[]);
drop function if exists get_handlers();
drop function if exists build_handler_param(text,jsonb);
drop type if exists get_handler_result_row;
drop type if exists get_handler_result_row_param;
drop table if exists handler_param;
drop table if exists handler;

create table handler (
  id int primary key generated by default as identity,
  method text not null,
  pattern text not null,
  sql text not null
);

create table handler_param (
  id int primary key generated by default as identity,
  handler_id int not null references handler,
  name text not null,
  position int not null,
  converters jsonb not null,
  unique(handler_id, name),
  unique(handler_id, position)
);

create function build_handler_param(
  _name text,
  _converters jsonb
) returns handler_param
language plpgsql as $$
declare
  _result handler_param;
begin
  _result.name = _name;
  _result.converters = _converters;

  return _result;
end;
$$;




create function create_handler(
  method text,
  pattern text,
  sql text,
  params handler_param[]
) returns handler
language plpgsql as $$
declare
  result handler;
  p handler_param;
  pos int = 0;
begin
  insert into handler (method, pattern, sql)
  values (method, pattern, sql)
  returning * into strict result;

  foreach p in array params
  loop
    pos = pos + 1;

    p.id = nextval('handler_param_id_seq');
    p.handler_id = result.id;
    p.position = pos;
    insert into handler_param values (p.*);
  end loop;

  return result;
end;
$$;

select create_handler(
  'GET',
  '/hey',
  'select json_build_object(''time'', now(), ''name'', $1::text)',
  array[
    build_handler_param('name', '[]'::jsonb)
  ]
);

create type get_handler_result_row_param as (
  name text,
  converters jsonb
);

create type get_handler_result_row as (
  method text,
  pattern text,
  sql text,
  params get_handler_result_row_param[]
);

create function get_handlers()
returns setof get_handler_result_row
language sql as $$
  select row(
    method,
    pattern,
    sql,
    (
      select coalesce(array_agg(row(name, converters)::get_handler_result_row_param order by position), '{}')
      from handler_param
      where handler_id=handler.id
    )
  )::get_handler_result_row
  from handler;
$$;
