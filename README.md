# Hannibal

Hannibal is an experimental web application server for PostgreSQL. It's goal was to push all application logic into the database and eliminate the need for a custom application server entirely.

It allows writing HTTP handlers entirely in SQL. e.g.

```sql
create function http_book_create(
  args jsonb,
  inout cookie_session jsonb,
  out status smallint,
  out response_headers jsonb,
  out template text,
  out template_data jsonb
)
language plpgsql as $$
declare
  _current_user users;
  _book_id int;
begin
  {{ template "require_current_user.sql" . }}

  if args -> '__errors__' is not null then
    template := 'book_new.html';
    template_data := jsonb_build_object(
      'currentUser', jsonb_build_object('id', _current_user.id, 'username', _current_user.username),
      'book', jsonb_build_object(
        'title', args -> 'title',
        'author', args -> 'author',
        'finishDate', args -> 'finishDate',
        'format', args -> 'format',
        'location', args -> 'location'
      ),
      'errors', args -> '__errors__'
    );

    return;
  end if;

  insert into books (user_id, title, author, finish_date, format, location)
  values (
    _current_user.id,
    (args ->> 'title'),
    (args ->> 'author'),
    (args ->> 'finishDate')::date,
    (args ->> 'format'),
    (args ->> 'location')
  )
  returning id into _book_id;

  status := 303;
  response_headers := jsonb_build_object('Location', format('/books/%s', _book_id));
end;
$$;
```

See https://github.com/jackc/booklogh for an example application.

## Status

Hannibal is not under active development. My conclusion after building this experimental server and using similar techniques elsewhere is that in most cases a separate application layer is preferable to pushing everything into the database.

This project is only made available for educational purposes.
