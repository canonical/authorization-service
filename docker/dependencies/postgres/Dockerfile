FROM postgres:14

# Copiamo i tre componenti dell'estensione
COPY pg_uuidv7.control /usr/share/postgresql/14/extension/
COPY pg_uuidv7--1.7.sql /usr/share/postgresql/14/extension/
COPY pg_uuidv7.so /usr/lib/postgresql/14/lib/

# Sistemiamo i permessi
RUN chown postgres:postgres /usr/share/postgresql/14/extension/pg_uuidv7* && \
    chown postgres:postgres /usr/lib/postgresql/14/lib/pg_uuidv7.so
