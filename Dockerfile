# =========================
# STAGE 1: Build Frontend (Vite)
# =========================
FROM node:20-alpine AS node-builder

WORKDIR /app

# Install dependencies
COPY package*.json ./
RUN npm install

# Copy source & build
COPY . .
RUN npm run build


# =========================
# STAGE 2: Laravel Runtime
# =========================
FROM php:8.3-cli-alpine

# Install system dependencies
RUN apk add --no-cache \
    bash \
    curl \
    libpng-dev \
    libjpeg-turbo-dev \
    freetype-dev \
    zip \
    unzip \
    git \
    oniguruma-dev

# Install PHP extensions
RUN docker-php-ext-configure gd \
    --with-freetype \
    --with-jpeg \
    && docker-php-ext-install \
    pdo \
    pdo_mysql \
    mbstring \
    exif \
    pcntl \
    bcmath \
    gd

# Install Composer
COPY --from=composer:2 /usr/bin/composer /usr/bin/composer

WORKDIR /var/www

# Copy project source
COPY . .

# Install Laravel dependencies (tanpa scripts biar aman dari error .env)
RUN composer install --no-dev --optimize-autoloader --no-scripts

# Copy hasil build Vite dari stage 1
COPY --from=node-builder /app/public/build /var/www/public/build

# Set permission
RUN chown -R www-data:www-data storage bootstrap/cache

# Expose port artisan serve
EXPOSE 8000

# Run Laravel
CMD php artisan serve --host=0.0.0.0 --port=8000
