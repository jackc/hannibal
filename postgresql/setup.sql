drop table if exists handlers;

create table handlers (
  method text not null,
  pattern text not null,
  sql text not null
);

insert into handlers (method, pattern, sql) values ('GET', '/hey', 'select json_build_object(''time'', now())');
