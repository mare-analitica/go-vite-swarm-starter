-- Development only: one role and database per system, never the superuser.
CREATE ROLE app LOGIN PASSWORD 'dev-app';
CREATE DATABASE app OWNER app;

CREATE ROLE n8n LOGIN PASSWORD 'dev-n8n';
CREATE DATABASE n8n OWNER n8n;
