create table todos (
  id int primary key generated by default as identity,
  name text not null
);
