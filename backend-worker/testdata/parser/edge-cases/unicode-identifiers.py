"""Module with non-ASCII identifiers to test unicode handling."""

# CJK identifiers
def 计算总和(数据列表):
    """Calculate the sum of a list of numbers."""
    总和 = 0
    for 数字 in 数据列表:
        总和 += 数字
    return 总和

# Accented Latin identifiers
def créer_utilisateur(prénom, nom_de_famille):
    """Create a user with accented names."""
    return {"prénom": prénom, "nom": nom_de_famille}

# Greek letter identifiers
def calculate_Δ(α, β, γ):
    """Calculate discriminant using Greek letters."""
    Δ = β ** 2 - 4 * α * γ
    return Δ

# Cyrillic identifiers
класс_данных = {"имя": "Тест", "значение": 42}

# Mixed script variable names
café_count = 3
naïve_approach = True
über_result = 计算总和([1, 2, 3])

# Emoji in strings (not identifiers, but tests string handling)
STATUS_MAP = {
    "success": "Operation completed",
    "error": "Something went wrong",
    "pending": "Waiting for result",
}

def résumé_parser(texte_entrée):
    """Parse a résumé from input text."""
    données = texte_entrée.strip().split("\n")
    résultat = []
    for ligne in données:
        if ligne:
            résultat.append(ligne)
    return résultat
