using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace GapuraAI.API.Models;

/// <summary>
/// Represents a registered internal application with its credentials and daily quota.
/// Maps to the 'Apps_Auth' MySQL table.
/// </summary>
[Table("Apps_Auth")]
public class AppAuth
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.Identity)]
    [Column("AppID")]
    public int AppId { get; set; }

    [Required]
    [StringLength(100)]
    [Column("ProjectName")]
    public string ProjectName { get; set; } = string.Empty;

    [Required]
    [StringLength(50)]
    [Column("Username")]
    public string Username { get; set; } = string.Empty;

    [Required]
    [StringLength(255)]
    [Column("PasswordHash")]
    public string PasswordHash { get; set; } = string.Empty;

    [Required]
    [Column("DailyTokenLimit")]
    public int DailyTokenLimit { get; set; } = 100_000;

    // Navigation property — one App can have many Audit Logs
    public ICollection<AuditLog> AuditLogs { get; set; } = new List<AuditLog>();
}
