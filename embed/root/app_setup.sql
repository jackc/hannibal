create function add(int, int) returns int
language sql as $$
  select $1 + $2;
$$;
