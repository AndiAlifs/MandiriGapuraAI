using GapuraAI.API.Data;
using Microsoft.EntityFrameworkCore;

var builder = WebApplication.CreateBuilder(args);

// ── MySQL / EF Core (Pomelo) ──────────────────────────────────────────
var connectionString = builder.Configuration.GetConnectionString("GapuraDb")
    ?? throw new InvalidOperationException("Connection string 'GapuraDb' not found.");

builder.Services.AddDbContext<GapuraDbContext>(options =>
    options.UseMySql(connectionString, ServerVersion.AutoDetect(connectionString)));

// ── HttpClient for OpenAI upstream ───────────────────────────────────
builder.Services.AddHttpClient("OpenAI", client =>
{
    client.BaseAddress = new Uri("https://api.openai.com/");
    client.Timeout = TimeSpan.FromSeconds(30);
    client.DefaultRequestHeaders.Add("Accept", "application/json");
});

// ── Controllers ──────────────────────────────────────────────────────
builder.Services.AddControllers();

// ── Swagger / OpenAPI (dev only) ─────────────────────────────────────
builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen(c =>
{
    c.SwaggerDoc("v1", new() { Title = "GAPURA AI Studio API", Version = "v1" });
});

// ── CORS (allow Angular dev server) ──────────────────────────────────
builder.Services.AddCors(options =>
{
    options.AddPolicy("AllowAngularDev", policy =>
    {
        policy.WithOrigins("http://localhost:4200")
              .AllowAnyHeader()
              .AllowAnyMethod();
    });
});

var app = builder.Build();

// ── Middleware Pipeline ──────────────────────────────────────────────
if (app.Environment.IsDevelopment())
{
    app.UseSwagger();
    app.UseSwaggerUI();
}

app.UseCors("AllowAngularDev");
app.MapControllers();

app.Run();
