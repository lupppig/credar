SELECT 'CREATE DATABASE kratos OWNER credar'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'kratos')\gexec
